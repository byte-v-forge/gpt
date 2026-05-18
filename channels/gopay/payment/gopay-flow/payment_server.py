#!/usr/bin/env python3
"""Segmented gRPC wrapper for the GoPay payment flow."""

from __future__ import annotations

import argparse
import copy
import logging
import threading
import uuid
from concurrent import futures
from dataclasses import dataclass
from typing import Any

import grpc

import payment_pb2
import payment_pb2_grpc
from gopay import (
    DEFAULT_STRIPE_PK,
    GoPayCharger,
    GoPayError,
    OTPCancelled,
    _build_chatgpt_session,
    _load_cfg,
    probe_plus_active_session_token,
    probe_plus_trial_checkout,
    resolve_gopay_cfg,
    resolve_checkout_proxy,
    resolve_payment_proxy,
    validate_gopay_cfg,
)

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    datefmt="%H:%M:%S",
)
logger = logging.getLogger(__name__)


def _billing_from_config(cfg: dict[str, Any]) -> dict[str, Any]:
    billing = cfg.get("billing") or {}
    if billing:
        return dict(billing)
    cards = cfg.get("cards") or []
    if cards and isinstance(cards[0], dict):
        card0 = cards[0]
        out = dict(card0.get("address") or {})
        for key in ("name", "email"):
            if card0.get(key):
                out[key] = card0[key]
        return out
    return {}


def _normalize_listen(value: str) -> str:
    value = (value or ":50051").strip()
    if value.startswith(":"):
        return "[::]" + value
    return value


def _close_session(session: Any) -> None:
    close = getattr(session, "close", None)
    if callable(close):
        try:
            close()
        except Exception:
            pass


def _looks_access_token(value: str) -> bool:
    # OpenAI access tokens are JWTs. NextAuth session tokens are JWE-like and
    # normally have a different dot count, so this is only a compatibility shim.
    return value.count(".") == 2


def _requires_manual_payment_confirmation(flow: "PendingFlow") -> bool:
    tokenization = str(getattr(flow.charger, "midtrans_tokenization", "true") or "true").strip().lower()
    return tokenization == "false"


@dataclass
class PendingFlow:
    charger: GoPayCharger
    state: dict[str, Any]
    use_account_token: bool = False

    def close(self) -> None:
        self.charger.close()


class FlowStore:
    def __init__(self):
        self._lock = threading.Lock()
        self._flows: dict[str, PendingFlow] = {}

    def put(self, charger: GoPayCharger, state: dict[str, Any], use_account_token: bool = False) -> str:
        flow_id = uuid.uuid4().hex
        with self._lock:
            self._flows[flow_id] = PendingFlow(charger=charger, state=state, use_account_token=use_account_token)
        return flow_id

    def get(self, flow_id: str) -> PendingFlow | None:
        with self._lock:
            return self._flows.get(flow_id)

    def pop(self, flow_id: str) -> PendingFlow | None:
        with self._lock:
            return self._flows.pop(flow_id, None)

    def close(self) -> None:
        with self._lock:
            flows = list(self._flows.values())
            self._flows.clear()
        for flow in flows:
            flow.close()


class PaymentService(payment_pb2_grpc.PaymentServiceServicer):
    def __init__(self, cfg: dict[str, Any]):
        self._cfg = cfg
        self._flows = FlowStore()

    def close(self) -> None:
        self._flows.close()

    def ProbeTier(self, request, context):
        session_token = (request.session_token or "").strip()
        if not session_token:
            return payment_pb2.ProbeTierPaymentResponse(
                success=False,
                error_message="session_token is required",
            )
        try:
            proxy = resolve_checkout_proxy(self._cfg)
            result = probe_plus_active_session_token(
                session_token,
                proxy=proxy,
                log=logger.info,
            )
            return payment_pb2.ProbeTierPaymentResponse(
                success=not bool(result.get("error_message")),
                error_message=str(result.get("error_message") or "")[:500],
                checked=bool(result.get("checked")),
                tier=str(result.get("tier") or result.get("plan_type") or ""),
                plus_active=bool(result.get("plus_active")),
                source=str(result.get("source") or "auth_session"),
            )
        except Exception as exc:
            logger.exception("[payment] ProbeTier crashed")
            return payment_pb2.ProbeTierPaymentResponse(success=False, error_message=str(exc)[:500])

    def ProbePlusTrial(self, request, context):
        session_token = (request.session_token or "").strip()
        access_token = (getattr(request, "access_token", "") or "").strip()
        if session_token and not access_token and _looks_access_token(session_token):
            access_token = session_token
            session_token = ""

        if not (session_token or access_token):
            return payment_pb2.ProbePlusTrialPaymentResponse(
                success=False,
                error_message="session_token or access_token is required",
            )

        cs_session = None
        try:
            cfg = copy.deepcopy(self._cfg)
            fresh_checkout = cfg.get("fresh_checkout") or {}
            auth_cfg = dict(fresh_checkout.get("auth") or {})
            auth_cfg.pop("cookie_header", None)
            auth_cfg.pop("session_token", None)
            auth_cfg.pop("access_token", None)
            if session_token:
                auth_cfg["session_token"] = session_token
            if access_token:
                auth_cfg["access_token"] = access_token

            proxy = resolve_checkout_proxy(cfg)
            session_probe = None
            if session_token:
                logger.info("[payment] ProbePlusSession start")
                session_probe = probe_plus_active_session_token(
                    session_token,
                    proxy=proxy,
                    log=logger.info,
                )
                if session_probe.get("checked") and session_probe.get("plus_active"):
                    logger.info(
                        "[payment] ProbePlusSession active plan=%s source=%s",
                        session_probe.get("plan_type"),
                        session_probe.get("source"),
                    )
                    return payment_pb2.ProbePlusTrialPaymentResponse(
                        success=True,
                        checked=True,
                        plus_trial_eligible=False,
                        source=str(session_probe.get("source") or "auth_session"),
                        plus_active=True,
                        plan_type=str(session_probe.get("tier") or session_probe.get("plan_type") or ""),
                    )
                if session_probe.get("error_message") and not access_token:
                    return payment_pb2.ProbePlusTrialPaymentResponse(
                        success=False,
                        checked=bool(session_probe.get("checked")),
                        error_message=str(session_probe.get("error_message") or "")[:500],
                        source=str(session_probe.get("source") or "auth_session"),
                        plus_active=False,
                        plan_type=str(session_probe.get("tier") or session_probe.get("plan_type") or ""),
                    )

            cs_session = _build_chatgpt_session(auth_cfg, proxy=proxy)
            stripe_pk = (
                (cfg.get("stripe") or {}).get("publishable_key")
                or auth_cfg.get("stripe_pk")
                or DEFAULT_STRIPE_PK
            )

            logger.info("[payment] ProbePlusTrial start")
            result = probe_plus_trial_checkout(
                cs_session,
                stripe_pk=stripe_pk,
                runtime_cfg=dict(cfg.get("runtime") or {}),
                checkout_cfg=dict((cfg.get("fresh_checkout") or {}).get("plan") or {}),
                proxy=proxy,
                log=logger.info,
            )
            logger.info(
                "[payment] ProbePlusTrial checked=%s eligible=%s amount=%s source=%s",
                result.get("checked"),
                result.get("plus_trial_eligible"),
                result.get("amount"),
                result.get("source"),
            )
            return payment_pb2.ProbePlusTrialPaymentResponse(
                success=True,
                error_message=str(result.get("error_message") or "")[:500],
                checked=bool(result.get("checked")),
                plus_trial_eligible=bool(result.get("plus_trial_eligible")),
                amount=int(result.get("amount") or 0),
                currency=str(result.get("currency") or ""),
                source=str(result.get("source") or ""),
                checkout_url=str(result.get("checkout_url") or ""),
                checkout_session_id=str(result.get("checkout_session_id") or ""),
                plus_active=bool(session_probe and session_probe.get("plus_active")),
                plan_type=str((session_probe or {}).get("tier") or (session_probe or {}).get("plan_type") or ""),
            )
        except GoPayError as exc:
            logger.error("[payment] ProbePlusTrial failed: %s", exc)
            return payment_pb2.ProbePlusTrialPaymentResponse(success=False, error_message=str(exc)[:500])
        except Exception as exc:
            logger.exception("[payment] ProbePlusTrial crashed")
            return payment_pb2.ProbePlusTrialPaymentResponse(success=False, error_message=str(exc)[:500])
        finally:
            if cs_session is not None:
                _close_session(cs_session)

    def CreateCheckoutLink(self, request, context):
        session_token = (request.session_token or "").strip()
        access_token = (getattr(request, "access_token", "") or "").strip()
        if session_token and not access_token and _looks_access_token(session_token):
            access_token = session_token
            session_token = ""
        if access_token:
            session_token = ""

        if not (session_token or access_token):
            return payment_pb2.CreateCheckoutLinkResponse(
                success=False,
                error_message="session_token or access_token is required",
            )

        charger = None
        cs_session = None
        try:
            cfg = copy.deepcopy(self._cfg)
            fresh_checkout = cfg.get("fresh_checkout") or {}
            auth_cfg = dict(fresh_checkout.get("auth") or {})
            auth_cfg.pop("cookie_header", None)
            auth_cfg.pop("session_token", None)
            auth_cfg.pop("access_token", None)
            if session_token:
                auth_cfg["session_token"] = session_token
            if access_token:
                auth_cfg["access_token"] = access_token

            checkout_proxy = resolve_checkout_proxy(cfg)
            cs_session = _build_chatgpt_session(auth_cfg, proxy=checkout_proxy)
            charger = GoPayCharger(
                cs_session,
                {"country_code": "0", "phone_number": "0", "pin": "0"},
                otp_provider=lambda: (_ for _ in ()).throw(OTPCancelled("OTP not used by checkout link")),
                checkout_proxy=checkout_proxy,
                payment_proxy=checkout_proxy,
                runtime_cfg=dict(cfg.get("runtime") or {}),
                checkout_cfg=dict((cfg.get("fresh_checkout") or {}).get("plan") or {}),
                log=logger.info,
            )

            logger.info("[payment] CreateCheckoutLink start")
            checkout_session_id = charger._chatgpt_create_checkout()
            checkout_url = charger.checkout_url or f"https://checkout.stripe.com/c/pay/{checkout_session_id}"
            logger.info(
                "[payment] CreateCheckoutLink created session_present=%s url_present=%s",
                bool(checkout_session_id),
                bool(checkout_url),
            )
            return payment_pb2.CreateCheckoutLinkResponse(
                success=True,
                checkout_url=str(checkout_url or ""),
                checkout_session_id=str(checkout_session_id or ""),
            )
        except GoPayError as exc:
            logger.error("[payment] CreateCheckoutLink failed: %s", exc)
            return payment_pb2.CreateCheckoutLinkResponse(success=False, error_message=str(exc)[:500])
        except Exception as exc:
            logger.exception("[payment] CreateCheckoutLink crashed")
            return payment_pb2.CreateCheckoutLinkResponse(success=False, error_message=str(exc)[:500])
        finally:
            if charger is not None:
                charger.close()
            elif cs_session is not None:
                _close_session(cs_session)

    def StartGoPay(self, request, context):
        session_token = (request.session_token or "").strip()
        access_token = (getattr(request, "access_token", "") or "").strip()
        use_account_token = bool(getattr(request, "use_account_token", False))
        tokenization = (getattr(request, "tokenization", "") or "").strip()
        checkout_url = (getattr(request, "checkout_url", "") or "").strip()
        checkout_session_id = (getattr(request, "checkout_session_id", "") or "").strip()
        gopay_phone = (getattr(request, "gopay_phone", "") or "").strip()
        otp_channel = (getattr(request, "otp_channel", "") or "").strip().lower()
        if session_token and not access_token and _looks_access_token(session_token):
            access_token = session_token
            session_token = ""
        if access_token:
            session_token = ""
        if use_account_token and not access_token:
            return payment_pb2.StartGoPayResponse(
                success=False,
                error_message="access_token is required when use_account_token=true",
            )
        if use_account_token and not gopay_phone:
            return payment_pb2.StartGoPayResponse(
                success=False,
                error_message="gopay_phone is required when use_account_token=true",
            )

        if not (session_token or access_token):
            return payment_pb2.StartGoPayResponse(
                success=False,
                error_message="session_token or access_token is required",
            )

        charger = None
        cs_session = None
        try:
            cfg = copy.deepcopy(self._cfg)
            fresh_checkout = cfg.get("fresh_checkout") or {}
            auth_cfg = dict(fresh_checkout.get("auth") or {})
            auth_cfg.pop("cookie_header", None)
            auth_cfg.pop("session_token", None)
            auth_cfg.pop("access_token", None)
            if session_token:
                auth_cfg["session_token"] = session_token
            if access_token:
                auth_cfg["access_token"] = access_token

            checkout_proxy = resolve_checkout_proxy(cfg)
            payment_proxy = resolve_payment_proxy(cfg)
            cs_session = _build_chatgpt_session(auth_cfg, proxy=checkout_proxy)

            gopay_cfg = resolve_gopay_cfg(cfg)
            if gopay_phone:
                gopay_cfg["phone_number"] = gopay_phone
            if tokenization:
                gopay_cfg["tokenization"] = tokenization
            gopay_cfg = validate_gopay_cfg(gopay_cfg)

            stripe_pk = (
                (cfg.get("stripe") or {}).get("publishable_key")
                or auth_cfg.get("stripe_pk")
                or DEFAULT_STRIPE_PK
            )
            runtime_cfg = dict(cfg.get("runtime") or {})
            charger = GoPayCharger(
                cs_session,
                gopay_cfg,
                otp_provider=lambda: (_ for _ in ()).throw(OTPCancelled("external OTP required")),
                checkout_proxy=checkout_proxy,
                payment_proxy=payment_proxy,
                runtime_cfg=runtime_cfg,
                checkout_cfg=dict((cfg.get("fresh_checkout") or {}).get("plan") or {}),
                browser_challenge_cfg=dict(cfg.get("browser_challenge") or {}),
                pre_solve_passive_captcha=bool(cfg.get("pre_solve_passive_captcha", False)),
                log=logger.info,
            )

            logger.info(
                "[payment] StartGoPay start reuse_checkout=%s otp_channel=%s",
                bool(checkout_session_id or checkout_url),
                otp_channel or "default",
            )
            state = charger.start_until_otp(
                stripe_pk=stripe_pk,
                billing=_billing_from_config(cfg),
                checkout_session_id=checkout_session_id,
                checkout_url=checkout_url,
                otp_channel=otp_channel,
            )
            flow_id = self._flows.put(charger, state, use_account_token=use_account_token)
            charger = None
            cs_session = None
            otp_required = bool(state.get("otp_required", True))
            logger.info(
                "[payment] StartGoPay flow=%s otp_required=%s",
                flow_id[:8],
                otp_required,
            )
            return payment_pb2.StartGoPayResponse(
                success=True,
                flow_id=flow_id,
                snap_token=str(state.get("snap_token") or ""),
                issued_after_unix=int(state.get("issued_after_unix") or 0),
                expires_at_unix=0,
                checkout_url=str(state.get("checkout_url") or ""),
                checkout_session_id=str(state.get("cs_id") or ""),
                otp_required=otp_required,
            )
        except GoPayError as exc:
            logger.error("[payment] StartGoPay failed: %s", exc)
            return payment_pb2.StartGoPayResponse(success=False, error_message=str(exc)[:500])
        except Exception as exc:
            logger.exception("[payment] StartGoPay crashed")
            return payment_pb2.StartGoPayResponse(success=False, error_message=str(exc)[:500])
        finally:
            if charger is not None:
                charger.close()
            elif cs_session is not None:
                _close_session(cs_session)

    def PrepareGoPay(self, request, context):
        session_token = (request.session_token or "").strip()
        access_token = (getattr(request, "access_token", "") or "").strip()
        tokenization = (getattr(request, "tokenization", "") or "").strip()
        checkout_url = (getattr(request, "checkout_url", "") or "").strip()
        checkout_session_id = (getattr(request, "checkout_session_id", "") or "").strip()
        gopay_phone = (getattr(request, "gopay_phone", "") or "").strip()
        if session_token and not access_token and _looks_access_token(session_token):
            access_token = session_token
            session_token = ""
        if access_token:
            session_token = ""
        if not (session_token or access_token):
            return payment_pb2.PrepareGoPayResponse(
                success=False,
                error_message="session_token or access_token is required",
            )

        charger = None
        cs_session = None
        try:
            cfg = copy.deepcopy(self._cfg)
            fresh_checkout = cfg.get("fresh_checkout") or {}
            auth_cfg = dict(fresh_checkout.get("auth") or {})
            auth_cfg.pop("cookie_header", None)
            auth_cfg.pop("session_token", None)
            auth_cfg.pop("access_token", None)
            if session_token:
                auth_cfg["session_token"] = session_token
            if access_token:
                auth_cfg["access_token"] = access_token

            checkout_proxy = resolve_checkout_proxy(cfg)
            payment_proxy = resolve_payment_proxy(cfg)
            cs_session = _build_chatgpt_session(auth_cfg, proxy=checkout_proxy)

            gopay_cfg = resolve_gopay_cfg(cfg)
            if gopay_phone:
                gopay_cfg["phone_number"] = gopay_phone
            if tokenization:
                gopay_cfg["tokenization"] = tokenization
            gopay_cfg = validate_gopay_cfg(gopay_cfg)

            stripe_pk = (
                (cfg.get("stripe") or {}).get("publishable_key")
                or auth_cfg.get("stripe_pk")
                or DEFAULT_STRIPE_PK
            )
            runtime_cfg = dict(cfg.get("runtime") or {})
            charger = GoPayCharger(
                cs_session,
                gopay_cfg,
                otp_provider=lambda: (_ for _ in ()).throw(OTPCancelled("external OTP required")),
                checkout_proxy=checkout_proxy,
                payment_proxy=payment_proxy,
                runtime_cfg=runtime_cfg,
                checkout_cfg=dict((cfg.get("fresh_checkout") or {}).get("plan") or {}),
                browser_challenge_cfg=dict(cfg.get("browser_challenge") or {}),
                pre_solve_passive_captcha=bool(cfg.get("pre_solve_passive_captcha", False)),
                log=logger.info,
            )

            logger.info(
                "[payment] PrepareGoPay start reuse_checkout=%s",
                bool(checkout_session_id or checkout_url),
            )
            state = charger.prepare_until_linking(
                stripe_pk=stripe_pk,
                billing=_billing_from_config(cfg),
                checkout_session_id=checkout_session_id,
                checkout_url=checkout_url,
            )
            flow_id = self._flows.put(charger, state, use_account_token=False)
            charger = None
            cs_session = None
            logger.info("[payment] PrepareGoPay flow=%s", flow_id[:8])
            return payment_pb2.PrepareGoPayResponse(
                success=True,
                flow_id=flow_id,
                snap_token=str(state.get("snap_token") or ""),
                checkout_url=str(state.get("checkout_url") or ""),
                checkout_session_id=str(state.get("cs_id") or ""),
            )
        except GoPayError as exc:
            logger.error("[payment] PrepareGoPay failed: %s", exc)
            return payment_pb2.PrepareGoPayResponse(success=False, error_message=str(exc)[:500])
        except Exception as exc:
            logger.exception("[payment] PrepareGoPay crashed")
            return payment_pb2.PrepareGoPayResponse(success=False, error_message=str(exc)[:500])
        finally:
            if charger is not None:
                charger.close()
            elif cs_session is not None:
                _close_session(cs_session)

    def StartPreparedGoPay(self, request, context):
        flow_id = (request.flow_id or "").strip()
        gopay_phone = (getattr(request, "gopay_phone", "") or "").strip()
        otp_channel = (getattr(request, "otp_channel", "") or "").strip().lower()
        if not flow_id:
            return payment_pb2.StartGoPayResponse(success=False, error_message="flow_id is required")

        flow = self._flows.get(flow_id)
        if flow is None:
            return payment_pb2.StartGoPayResponse(success=False, error_message="prepared payment flow not found")

        try:
            logger.info(
                "[payment] StartPreparedGoPay flow=%s otp_channel=%s",
                flow_id[:8],
                otp_channel or "default",
            )
            state = flow.charger.start_prepared_linking_until_otp(
                flow.state,
                otp_channel=otp_channel,
                gopay_phone=gopay_phone,
            )
            flow.state = state
            otp_required = bool(state.get("otp_required", True))
            logger.info(
                "[payment] StartPreparedGoPay flow=%s otp_required=%s",
                flow_id[:8],
                otp_required,
            )
            return payment_pb2.StartGoPayResponse(
                success=True,
                flow_id=flow_id,
                snap_token=str(state.get("snap_token") or ""),
                issued_after_unix=int(state.get("issued_after_unix") or 0),
                expires_at_unix=0,
                checkout_url=str(state.get("checkout_url") or ""),
                checkout_session_id=str(state.get("cs_id") or ""),
                otp_required=otp_required,
            )
        except GoPayError as exc:
            logger.error("[payment] StartPreparedGoPay failed: %s", exc)
            failed = self._flows.pop(flow_id)
            if failed is not None:
                failed.close()
            return payment_pb2.StartGoPayResponse(success=False, error_message=str(exc)[:500])
        except Exception as exc:
            logger.exception("[payment] StartPreparedGoPay crashed")
            failed = self._flows.pop(flow_id)
            if failed is not None:
                failed.close()
            return payment_pb2.StartGoPayResponse(success=False, error_message=str(exc)[:500])

    def ResendGoPayOTP(self, request, context):
        flow_id = (request.flow_id or "").strip()
        if not flow_id:
            return payment_pb2.ResendGoPayOTPResponse(success=False, error_message="flow_id is required")

        flow = self._flows.get(flow_id)
        if flow is None:
            return payment_pb2.ResendGoPayOTPResponse(success=False, error_message="payment flow not found")

        try:
            logger.info("[payment] ResendGoPayOTP flow=%s", flow_id[:8])
            state = flow.charger.resend_linking_otp(flow.state)
            flow.state = state
            return payment_pb2.ResendGoPayOTPResponse(
                success=True,
                flow_id=flow_id,
                issued_after_unix=int(state.get("issued_after_unix") or 0),
            )
        except GoPayError as exc:
            logger.error("[payment] ResendGoPayOTP failed: %s", exc)
            return payment_pb2.ResendGoPayOTPResponse(success=False, flow_id=flow_id, error_message=str(exc)[:500])
        except Exception as exc:
            logger.exception("[payment] ResendGoPayOTP crashed")
            return payment_pb2.ResendGoPayOTPResponse(success=False, flow_id=flow_id, error_message=str(exc)[:500])

    def CompleteGoPay(self, request, context):
        if not request.flow_id:
            return payment_pb2.GoPayResponse(success=False, error_message="flow_id is required")

        flow = self._flows.get(request.flow_id)
        if flow is None:
            return payment_pb2.GoPayResponse(success=False, error_message="payment flow not found")
        if not request.otp and bool(flow.state.get("otp_required", True)):
            return payment_pb2.GoPayResponse(success=False, error_message="otp is required")

        close_flow = True
        try:
            logger.info("[payment] CompleteGoPay flow=%s", request.flow_id[:8])
            if _requires_manual_payment_confirmation(flow):
                if bool(flow.state.get("otp_required", True)):
                    result = flow.charger.complete_after_otp_until_manual_confirmation(flow.state, request.otp)
                    flow.state = result
                else:
                    result = flow.state
                flow.state = result
                close_flow = False
                return payment_pb2.GoPayResponse(
                    success=True,
                    awaiting_manual_confirmation=True,
                    charge_ref=str(result.get("charge_ref") or ""),
                    snap_token=str(result.get("snap_token") or ""),
                    deeplink_url=str(result.get("deeplink_url") or ""),
                    qr_code_url=str(result.get("qr_code_url") or ""),
                    finish_redirect_url=str(result.get("finish_redirect_url") or ""),
                    finish_200_redirect_url=str(result.get("finish_200_redirect_url") or ""),
                )

            if bool(flow.state.get("otp_required", True)):
                result = flow.charger.complete_after_otp(flow.state, request.otp)
            else:
                result = flow.charger.complete_after_manual_confirmation(flow.state)
            state = str(result.get("state") or "")
            success = state == "succeeded"
            return payment_pb2.GoPayResponse(
                success=success,
                error_message="" if success else f"payment state={state or 'unknown'}",
                charge_ref=str(result.get("charge_ref") or ""),
                snap_token=str(result.get("snap_token") or ""),
                deeplink_url=str(result.get("deeplink_url") or ""),
                qr_code_url=str(result.get("qr_code_url") or ""),
                finish_redirect_url=str(result.get("finish_redirect_url") or ""),
                finish_200_redirect_url=str(result.get("finish_200_redirect_url") or ""),
            )
        except GoPayError as exc:
            logger.error("[payment] CompleteGoPay failed: %s", exc)
            return payment_pb2.GoPayResponse(success=False, error_message=str(exc)[:500])
        except Exception as exc:
            logger.exception("[payment] CompleteGoPay crashed")
            return payment_pb2.GoPayResponse(success=False, error_message=str(exc)[:500])
        finally:
            if close_flow:
                flow = self._flows.pop(request.flow_id)
                if flow is not None:
                    flow.close()

    def ConfirmGoPayPayment(self, request, context):
        if not request.flow_id:
            return payment_pb2.GoPayResponse(success=False, error_message="flow_id is required")

        flow = self._flows.pop(request.flow_id)
        if flow is None:
            return payment_pb2.GoPayResponse(success=False, error_message="payment flow not found")

        try:
            logger.info("[payment] ConfirmGoPayPayment flow=%s", request.flow_id[:8])
            result = flow.charger.complete_after_manual_confirmation(flow.state)
            state = str(result.get("state") or "")
            success = state == "succeeded"
            return payment_pb2.GoPayResponse(
                success=success,
                error_message="" if success else f"payment state={state or 'unknown'}",
                charge_ref=str(result.get("charge_ref") or ""),
                snap_token=str(result.get("snap_token") or ""),
                deeplink_url=str(result.get("deeplink_url") or ""),
                qr_code_url=str(result.get("qr_code_url") or ""),
                finish_redirect_url=str(result.get("finish_redirect_url") or ""),
                finish_200_redirect_url=str(result.get("finish_200_redirect_url") or ""),
            )
        except GoPayError as exc:
            logger.error("[payment] ConfirmGoPayPayment failed: %s", exc)
            return payment_pb2.GoPayResponse(success=False, error_message=str(exc)[:500])
        except Exception as exc:
            logger.exception("[payment] ConfirmGoPayPayment crashed")
            return payment_pb2.GoPayResponse(success=False, error_message=str(exc)[:500])
        finally:
            flow.close()

    def CancelGoPay(self, request, context):
        if not request.flow_id:
            return payment_pb2.CancelGoPayResponse(success=False, error_message="flow_id is required")
        flow = self._flows.pop(request.flow_id)
        if flow is not None:
            flow.close()
        logger.info("[payment] CancelGoPay flow=%s found=%s", request.flow_id[:8], flow is not None)
        return payment_pb2.CancelGoPayResponse(success=True)


def serve(config_path: str, listen: str):
    cfg = _load_cfg(config_path)
    service = PaymentService(cfg)
    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=4),
        options=[
            ("grpc.max_send_message_length", 50 * 1024 * 1024),
            ("grpc.max_receive_message_length", 50 * 1024 * 1024),
        ],
    )
    payment_pb2_grpc.add_PaymentServiceServicer_to_server(service, server)
    listen_addr = _normalize_listen(listen)
    server.add_insecure_port(listen_addr)
    server.start()
    logger.info("[payment] gRPC listening on %s", listen_addr)
    try:
        server.wait_for_termination()
    except KeyboardInterrupt:
        logger.info("[payment] shutting down")
        server.stop(grace=5)
    finally:
        service.close()


def main():
    parser = argparse.ArgumentParser(description="GoPay payment gRPC service")
    parser.add_argument("--config", default="config.json")
    parser.add_argument("--listen", default=":50051")
    parser.add_argument("--flow-ttl", type=int, default=0, help=argparse.SUPPRESS)
    args = parser.parse_args()

    serve(
        config_path=args.config,
        listen=args.listen,
    )


if __name__ == "__main__":
    main()
