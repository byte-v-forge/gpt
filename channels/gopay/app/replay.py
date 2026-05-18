"""Direct Midtrans GoPay link payment executor for GoPay App."""

from __future__ import annotations

import base64
import json
import re
import time
import uuid
import zlib
from dataclasses import dataclass
from typing import Any


GOPAY_CUSTOMER = "https://customer.gopayapi.com"
DEFAULT_CURRENCY = "IDR"
DEFAULT_SERVICE_ID = "1001"
MIDTRANS_MERCHANT_TRANSFER_SERVICE_ID = "1002"


@dataclass
class LinkPaymentOptions:
    payment_link: str = ""
    pin: str = ""
    amount_value: int = 1
    amount_currency: str = DEFAULT_CURRENCY
    body_limit: int = 1200


@dataclass
class LinkedAppUnlinkOptions:
    body_limit: int = 1200


def run_linked_app_unlink(client, options: LinkedAppUnlinkOptions | None = None) -> dict[str, Any]:
    """Execute the app-side linked-app unlink action."""
    options = options or LinkedAppUnlinkOptions()
    steps: list[dict[str, Any]] = []
    try:
        linkedapps = _call(
            steps,
            "linkedapps",
            lambda: client.get(f"{GOPAY_CUSTOMER}/v1/linkedapps"),
            options,
        )
        _require_status("linkedapps", linkedapps, {200})

        services = extract_linked_services(linkedapps)
        unlinked_count = 0
        failed: list[str] = []
        for service in services:
            unlink_path = str(service.get("unlink_service_url") or "").strip()
            if not unlink_path:
                continue
            name = str(service.get("service_name") or service.get("name") or unlink_path)
            response = _call(
                steps,
                f"unlink:{name}",
                lambda path=unlink_path: client.patch(
                    f"{GOPAY_CUSTOMER}{path}",
                ),
                options,
            )
            if int(response.get("status") or 0) in {200, 202, 204}:
                unlinked_count += 1
            else:
                failed.append(name)

        if failed:
            return {
                "success": False,
                "error_message": f"unlink failed: {', '.join(failed)}",
                "unlinked_count": unlinked_count,
                "steps": steps,
            }
        return {
            "success": True,
            "error_message": "",
            "unlinked_count": unlinked_count,
            "steps": steps,
        }
    except Exception as exc:
        return {
            "success": False,
            "error_message": str(exc),
            "unlinked_count": 0,
            "steps": steps,
        }


def run_link_payment(client, options: LinkPaymentOptions) -> dict[str, Any]:
    """Execute the app-side Midtrans GoPay link payment flow using the DB-backed token."""
    steps: list[dict[str, Any]] = []
    try:
        if not options.pin:
            raise ValueError("pin required")
        payment_ref = extract_midtrans_payment_ref(options.payment_link)
        detail = _call(
            steps,
            "payment_detail",
            lambda: client.get(
                f"{GOPAY_CUSTOMER}/customers/v1/payments/{payment_ref}?fetch_promotion_details=false",
            ),
            options,
        )
        _require_status("payment_detail", detail, {200})
        order = extract_payment_order(detail)
        order.setdefault("payment_id", payment_ref)
        return run_payment_order(client, order, options.pin, steps=steps, options=options)
    except Exception as exc:
        return {
            "success": False,
            "error_message": str(exc),
            "payment_id": "",
            "status": "",
            "steps": steps,
        }


_MIDTRANS_PAYMENT_REF_RE = re.compile(r"A[0-9]{12,}[A-Za-z0-9]+ID")
MIDTRANS_MERCHANT_PAN = "9360091437614825878"
MIDTRANS_QRIS_MERCHANT_PAN = "936009143761482587"
MIDTRANS_MERCHANT_ID = "G761482587"
MIDTRANS_MERCHANT_NAME = "OpenAI LLC"
MIDTRANS_MERCHANT_CITY = "JAKARTA SELATAN"
MIDTRANS_MERCHANT_POSTAL_CODE = "12190"
MIDTRANS_MERCHANT_CRITERIA = "UBE"
MIDTRANS_MERCHANT_CATEGORY_CODE = "5817"
MIDTRANS_TERMINAL_LABEL = "A01"
MIDTRANS_ACQUIRER_ID = MIDTRANS_MERCHANT_PAN[:8]
MIDTRANS_CURRENCY_CODE = "360"
MIDTRANS_QRIS_DOMAIN = "ID.CO.QRIS.WWW"
MIDTRANS_QRIS_NMID = "ID2025455428081"
MIDTRANS_MERCHANT_CROSS_REFERENCE_ID = "2e8eb8ee-4f94-4c68-8792-cadd46e98d22"


def extract_midtrans_payment_ref(payment_link: str) -> str:
    text = str(payment_link or "").strip()
    match = _MIDTRANS_PAYMENT_REF_RE.search(text)
    if not match:
        raise ValueError("midtrans payment reference missing from payment_link")
    return match.group(0)


def build_midtrans_link_order(
    payment_link: str,
    *,
    amount_value: int = 1,
    amount_currency: str = DEFAULT_CURRENCY,
) -> dict[str, Any]:
    payment_ref = extract_midtrans_payment_ref(payment_link)
    amount = int(amount_value or 1)
    if amount <= 0:
        raise ValueError("amount_value must be positive")
    currency = amount_currency or DEFAULT_CURRENCY
    additional = {"07": MIDTRANS_TERMINAL_LABEL, "50": payment_ref}
    additional_data_national = build_additional_data_national(MIDTRANS_MERCHANT_POSTAL_CODE, additional)
    cross_reference_id = MIDTRANS_MERCHANT_CROSS_REFERENCE_ID
    checksum = {"version": "3", "value": str(zlib.crc32(payment_ref.encode()) & 0xffffffff)}
    aspi_qr_data = {
        "amount": amount,
        "postal_code": MIDTRANS_MERCHANT_POSTAL_CODE,
        "merchant_city": MIDTRANS_MERCHANT_CITY,
        "merchant_id": MIDTRANS_MERCHANT_ID,
        "merchant_criteria": MIDTRANS_MERCHANT_CRITERIA,
        "merchant_pan": MIDTRANS_MERCHANT_PAN,
        "country_code": "ID",
        "transaction_currency_code": MIDTRANS_CURRENCY_CODE,
        "additional_data_national": additional_data_national,
        "additional_data": {
            "store_label": None,
            "beneficiary_account_name": None,
            "mobile_number": None,
            "reference_label": None,
            "customer_pan": None,
            "beneficiary_account_type": None,
            "purpose_of_transaction": None,
            "customer_label": None,
            "terminal_label": MIDTRANS_TERMINAL_LABEL,
            "bill_number": None,
            "custom_50": payment_ref,
            "additional_consumer_data_request": None,
            "customer_name": None,
            "loyalty_number": None,
        },
        "merchant_category_code": MIDTRANS_MERCHANT_CATEGORY_CODE,
        "merchant_name": MIDTRANS_MERCHANT_NAME,
        "trx_fee_amount": 0,
        "acquirer_id": MIDTRANS_ACQUIRER_ID,
    }
    return {
        "amount": {"value": amount, "currency": currency},
        "channel_type": "ONLINE_GATEWAY",
        "checksum": checksum,
        "payment_id": payment_ref,
        "payment_widget_metadata": {
            "service_id": DEFAULT_SERVICE_ID,
            "merchant_id": MIDTRANS_MERCHANT_ID,
        },
        "additional_data": {
            "aspiqr_information": {
                "qr_transaction_type": "ON-US",
                "merchant_pan": MIDTRANS_MERCHANT_PAN,
                "transaction_currency_code": MIDTRANS_CURRENCY_CODE,
                "trx_fee_amount": 0,
                "issuer_name": "gopay",
                "issuer_id": MIDTRANS_ACQUIRER_ID,
                "acquirer_name": "gopay",
                "acquirer_id": MIDTRANS_ACQUIRER_ID,
                "merchant_id": MIDTRANS_MERCHANT_ID,
                "merchant_name": MIDTRANS_MERCHANT_NAME,
                "merchant_city": MIDTRANS_MERCHANT_CITY,
                "merchant_criteria": MIDTRANS_MERCHANT_CRITERIA,
                "merchant_category_code": MIDTRANS_MERCHANT_CATEGORY_CODE,
                "postal_code": MIDTRANS_MERCHANT_POSTAL_CODE,
                "country_code": "ID",
                "terminal_label": MIDTRANS_TERMINAL_LABEL,
                "presentation_mode": "MPM",
                "additional_data_national": additional_data_national,
            },
            "aspiqr_information_v2": {
                "network_participant": {
                    "issuer": {"id": MIDTRANS_ACQUIRER_ID, "name": "gopay"},
                    "acquirer": {"id": MIDTRANS_ACQUIRER_ID, "name": "gopay"},
                },
                "merchant_information": {
                    "id": MIDTRANS_MERCHANT_ID,
                    "name": MIDTRANS_MERCHANT_NAME,
                    "city": MIDTRANS_MERCHANT_CITY,
                    "postal_code": MIDTRANS_MERCHANT_POSTAL_CODE,
                    "criteria": MIDTRANS_MERCHANT_CRITERIA,
                    "category_code": MIDTRANS_MERCHANT_CATEGORY_CODE,
                    "country_code": "ID",
                    "terminal_id": MIDTRANS_TERMINAL_LABEL,
                    "sub_merchant_id": "",
                    "external_store_id": "",
                },
                "transaction_identification": {
                    "transaction_type": "ON-US",
                    "presentation_mode": "MPM",
                    "type": "ON-US",
                },
                "account_participant": {
                    "to_account": MIDTRANS_MERCHANT_PAN,
                },
                "transaction_details": {
                    "amount": {"value": amount, "currency_code": currency},
                },
                "mpm_additional_data": {
                    "additional_consumer_data_request": "",
                    "additional_data_national": additional_data_national,
                    "bill_number": "",
                    "customer_label": "",
                    "loyalty_number": "",
                    "mobile_number": "",
                    "purpose_of_transaction": "",
                    "reference_label": "",
                    "store_label": "",
                    "terminal_label": MIDTRANS_TERMINAL_LABEL,
                },
            },
            "merchant_information": {
                "name": MIDTRANS_MERCHANT_NAME,
                "address": "",
                "brand_id": "PARTNER-OWNED-716",
                "id": MIDTRANS_MERCHANT_ID,
                "cross_reference_id": cross_reference_id,
                "tags": "gopay_merchants_all_tag,gopay_online_merchant",
                "city": MIDTRANS_MERCHANT_CITY,
                "postal_code": MIDTRANS_MERCHANT_POSTAL_CODE,
                "acquired_by": "MIDTRANS",
                "partner_id": "170630044941123400001",
                "user_id": "170630052010170000200001",
                "terminal_id": MIDTRANS_TERMINAL_LABEL,
                "merchant_category_code": MIDTRANS_MERCHANT_CATEGORY_CODE,
                "merchant_criteria": MIDTRANS_MERCHANT_CRITERIA,
            },
            "customer_flow": "qr",
        },
        "metadata": {
            "checksum": json_compact(checksum),
            "merchant_cross_reference_id": cross_reference_id,
            "external_merchant_name": MIDTRANS_MERCHANT_NAME,
            "payment_widget_intent": "EWALLET_QR",
            "aspi_qr_data": json_compact(aspi_qr_data),
            "aspi_qr_transaction_type": "ON-US",
            "aspi_qr_issuer": "gopay",
            "aspi_qr_acquirer": "gopay",
            "channel_type": "ONLINE_GATEWAY",
            "customer_flow": "qr",
            "internal_service": "snap",
            "internal_source": "partner-api",
            "internal_source_version": "2.3.0",
            "internal_reminder_type": "PENDING_PAYMENT",
            "service_type": "QRIS",
            "service_id": DEFAULT_SERVICE_ID,
            "tags": '{ "service_type": "GOPAY_OFFLINE" }',
        },
    }


def build_midtrans_link_qris_body(
    payment_link: str,
    *,
    amount_value: int = 1,
    amount_currency: str = DEFAULT_CURRENCY,
) -> dict[str, Any]:
    payment_ref = extract_midtrans_payment_ref(payment_link)
    amount = int(amount_value or 1)
    if amount <= 0:
        raise ValueError("amount_value must be positive")
    currency = amount_currency or DEFAULT_CURRENCY
    qr_code = build_midtrans_qris_code(payment_ref, amount)
    additional = {"07": MIDTRANS_TERMINAL_LABEL, "50": payment_ref}
    additional_data_national = build_additional_data_national(MIDTRANS_MERCHANT_POSTAL_CODE, additional)
    checksum = {"version": "3", "value": str(zlib.crc32(qr_code.encode()) & 0xffffffff)}
    aspi_qr_data = {
        "amount": amount,
        "postal_code": MIDTRANS_MERCHANT_POSTAL_CODE,
        "merchant_city": MIDTRANS_MERCHANT_CITY,
        "merchant_id": MIDTRANS_MERCHANT_ID,
        "merchant_criteria": MIDTRANS_MERCHANT_CRITERIA,
        "merchant_pan": MIDTRANS_MERCHANT_PAN,
        "country_code": "ID",
        "transaction_currency_code": MIDTRANS_CURRENCY_CODE,
        "additional_data_national": additional_data_national,
        "additional_data": {
            "store_label": None,
            "beneficiary_account_name": None,
            "mobile_number": None,
            "reference_label": None,
            "customer_pan": None,
            "beneficiary_account_type": None,
            "purpose_of_transaction": None,
            "customer_label": None,
            "terminal_label": MIDTRANS_TERMINAL_LABEL,
            "bill_number": None,
            "custom_50": payment_ref,
            "additional_consumer_data_request": None,
            "customer_name": None,
            "loyalty_number": None,
        },
        "merchant_category_code": MIDTRANS_MERCHANT_CATEGORY_CODE,
        "merchant_name": MIDTRANS_MERCHANT_NAME,
        "trx_fee_amount": 0,
        "acquirer_id": MIDTRANS_ACQUIRER_ID,
    }
    return {
        "qr_code": qr_code,
        "amount": {"value": amount, "currency": currency},
        "channel_type": "DYNAMIC_QR",
        "additional_data": {
            "merchant_order_id": "",
            "aspiqr_information": {
                "qr_transaction_type": "ON-US",
                "merchant_pan": MIDTRANS_MERCHANT_PAN,
                "transaction_currency_code": MIDTRANS_CURRENCY_CODE,
                "trx_fee_amount": 0,
                "retrieval_reference_number": "",
                "issuer_name": "gopay",
                "issuer_id": MIDTRANS_ACQUIRER_ID,
                "acquirer_name": "gopay",
                "acquirer_id": MIDTRANS_ACQUIRER_ID,
                "merchant_id": MIDTRANS_MERCHANT_ID,
                "merchant_name": MIDTRANS_MERCHANT_NAME,
                "merchant_city": MIDTRANS_MERCHANT_CITY,
                "merchant_criteria": MIDTRANS_MERCHANT_CRITERIA,
                "merchant_category_code": MIDTRANS_MERCHANT_CATEGORY_CODE,
                "postal_code": MIDTRANS_MERCHANT_POSTAL_CODE,
                "country_code": "ID",
                "additional_data_national": additional_data_national,
                "store_label": "",
                "mobile_number": "",
                "reference_label": "",
                "purpose_of_transaction": "",
                "customer_label": "",
                "terminal_label": MIDTRANS_TERMINAL_LABEL,
                "bill_number": "",
                "additional_consumer_data_request": "",
                "loyalty_number": "",
                "presentation_mode": "",
                "customer_pan": "",
                "customer_name": "",
                "beneficiary_account_name": "",
                "beneficiary_account_type": "DEFAULT",
            },
            "aspiqr_information_v2": {
                "network_participant": {
                    "issuer": {"id": MIDTRANS_ACQUIRER_ID, "name": "gopay"},
                    "acquirer": {"id": MIDTRANS_ACQUIRER_ID, "name": "gopay"},
                    "switch": {"id": "", "name": ""},
                },
                "account_participant": {
                    "from_account": "",
                    "to_account": MIDTRANS_MERCHANT_PAN,
                },
                "customer_information": {"name": "", "language_preference": ""},
                "merchant_information": {
                    "id": MIDTRANS_MERCHANT_ID,
                    "name": MIDTRANS_MERCHANT_NAME,
                    "city": MIDTRANS_MERCHANT_CITY,
                    "postal_code": MIDTRANS_MERCHANT_POSTAL_CODE,
                    "criteria": MIDTRANS_MERCHANT_CRITERIA,
                    "category_code": MIDTRANS_MERCHANT_CATEGORY_CODE,
                    "country_code": "ID",
                    "terminal_id": MIDTRANS_TERMINAL_LABEL,
                    "sub_merchant_id": "",
                    "external_store_id": "",
                },
                "transaction_identification": {
                    "presentation_mode": "MPM",
                    "type": "ON-US",
                    "retrieval_reference_number": "",
                    "invoice_number": "",
                },
                "transaction_details": {
                    "amount": {"value": amount, "currency": currency},
                    "fee_amount": {"value": 0, "currency": ""},
                },
                "mpm_additional_data": {
                    "additional_data_national": additional_data_national,
                    "bill_number": "",
                    "mobile_number": "",
                    "store_label": "",
                    "loyalty_number": "",
                    "reference_label": "",
                    "customer_label": "",
                    "terminal_label": MIDTRANS_TERMINAL_LABEL,
                    "purpose_of_transaction": "",
                    "additional_consumer_data_request": "",
                },
                "cpm_additional_data": {
                    "qr_data": "",
                    "flow_type": "",
                    "completion_product_indicator": "",
                    "scanner_information": {"id": "", "version": "", "model": "", "ip_address": ""},
                },
            },
            "customer_flow": "qr",
        },
        "metadata": {
            "checksum": json_compact(checksum),
            "merchant_cross_reference_id": MIDTRANS_MERCHANT_CROSS_REFERENCE_ID,
            "external_merchant_name": MIDTRANS_MERCHANT_NAME,
            "payment_widget_intent": "DYNAMIC_QR",
            "aspi_qr_data": json_compact(aspi_qr_data),
            "aspi_qr_transaction_type": "ON-US",
            "aspi_qr_issuer": "gopay",
            "aspi_qr_acquirer": "gopay",
            "customer_flow": "qr",
            "qris_source_info": build_qris_source_info(),
        },
    }


def qris_request_debug(body: dict[str, Any]) -> dict[str, Any]:
    metadata = body.get("metadata") if isinstance(body.get("metadata"), dict) else {}
    qr_code = str(body.get("qr_code") or "")
    payment_ref = ""
    try:
        payment_ref = extract_midtrans_payment_ref(qr_code)
    except Exception:
        pass
    return {
        "payment_ref": payment_ref,
        "amount": body.get("amount"),
        "channel_type": body.get("channel_type"),
        "qr_crc_tail": qr_code[-8:] if qr_code else "",
        "metadata_checksum": metadata.get("checksum"),
        "merchant_cross_reference_id": metadata.get("merchant_cross_reference_id"),
        "qris_source_info": metadata.get("qris_source_info"),
    }


def build_midtrans_qris_code(payment_ref: str, amount: int) -> str:
    merchant_account = encode_tlv(
        [
            ("00", "COM.GO-JEK.WWW"),
            ("01", MIDTRANS_QRIS_MERCHANT_PAN),
            ("02", MIDTRANS_MERCHANT_ID),
            ("03", MIDTRANS_MERCHANT_CRITERIA),
        ]
    )
    qris_account = encode_tlv(
        [
            ("00", MIDTRANS_QRIS_DOMAIN),
            ("02", MIDTRANS_QRIS_NMID),
            ("03", MIDTRANS_MERCHANT_CRITERIA),
        ]
    )
    additional = encode_tlv([("50", payment_ref), ("07", MIDTRANS_TERMINAL_LABEL)])
    body = encode_tlv(
        [
            ("00", "01"),
            ("01", "12"),
            ("26", merchant_account),
            ("51", qris_account),
            ("52", MIDTRANS_MERCHANT_CATEGORY_CODE),
            ("53", MIDTRANS_CURRENCY_CODE),
            ("54", str(amount)),
            ("58", "ID"),
            ("59", MIDTRANS_MERCHANT_NAME),
            ("60", MIDTRANS_MERCHANT_CITY),
            ("61", MIDTRANS_MERCHANT_POSTAL_CODE),
            ("62", additional),
        ]
    )
    return body + "6304" + f"{crc16_ccitt((body + '6304').encode('ascii')):04X}"


def build_qris_source_info() -> str:
    now_ms = int(time.time() * 1000)
    image_path = f"/data/user/0/com.gojek.gopay/cache/{uuid.uuid4()}/qr-code.png"
    return json_compact(
        {
            "image": (
                "IMAGE_NAME:qr-code.png|"
                f"IMAGE_GALLERY_LOCATION:{image_path}|"
                "IMAGE_FILE_TYPE:image/png|IMAGE_SIZE:1.74 KB|IMAGE_RESOLUTION:630.0_630.0|"
                f"TIME_OF_DOWNLOADED_IMAGE_IN_TIMESTAMP_MILLISECONDS:{now_ms}"
            ),
            "source": "GALLERY",
            "upload_time": str(now_ms),
        }
    )


def run_payment_order(
    client,
    order: dict[str, Any],
    pin: str,
    *,
    steps: list[dict[str, Any]] | None = None,
    options: LinkPaymentOptions | None = None,
) -> dict[str, Any]:
    options = options or LinkPaymentOptions(pin=pin)
    steps = steps if steps is not None else []
    payment_id = str(order.get("payment_id") or "").strip()
    if not payment_id:
        raise ValueError("payment_id missing from order")

    checkout = _call(
        steps,
        "checkout_list",
        lambda: client.post(
            f"{GOPAY_CUSTOMER}/v2/customer/payment-options/checkout/list",
            body=build_checkout_body(order, service_id=MIDTRANS_MERCHANT_TRANSFER_SERVICE_ID),
        ),
        options,
    )
    _require_status("checkout_list", checkout, {200})
    payment_token = extract_payment_token(checkout)

    promotions = _call(
        steps,
        "promotions_evaluate",
        lambda: client.post(
            f"{GOPAY_CUSTOMER}/v1/promotions/evaluate",
            body=build_promotions_evaluate_body(order, payment_token),
        ),
        options,
    )
    _require_status("promotions_evaluate", promotions, {200})

    capture_payment_token = randomize_payment_option_token(payment_token)
    capture_body = build_capture_body(order, capture_payment_token)
    capture1 = _call(
        steps,
        "capture1",
        lambda: client.patch(
            f"{GOPAY_CUSTOMER}/v3/payments/{payment_id}/capture",
            body=capture_body,
            extra_headers={"Idempotency-Key": str(uuid.uuid1())},
        ),
        options,
    )
    challenge_id, client_id = extract_challenge(capture1)

    last_used = _call(
        steps,
        "last_used",
        lambda: client.put(
            f"{GOPAY_CUSTOMER}/v1/customer/payment-options/settings/last-used",
            body={"token": payment_token},
        ),
        options,
    )
    _require_status("last_used", last_used, {200})

    _call(
        steps,
        "pin_page",
        lambda: client.get(f"{GOPAY_CUSTOMER}/api/v2/challenges/{challenge_id}/pin-page"),
        options,
    )

    pin_resp = _call(
        steps,
        "pin_tokens",
        lambda: client.post(
            f"{GOPAY_CUSTOMER}/api/v1/users/pin/tokens",
            body={
                "pin": pin,
                "client_id": client_id,
                "challenge_id": challenge_id,
            },
        ),
        options,
    )
    _require_status("pin_tokens", pin_resp, {200})
    pin_token = extract_pin_token(pin_resp)

    capture2_body = build_capture_body(
        order,
        capture_payment_token,
        pin_token=pin_token,
        challenge_id=challenge_id,
        client_id=client_id,
    )
    capture2 = _call(
        steps,
        "capture2",
        lambda: client.patch(
            f"{GOPAY_CUSTOMER}/v3/payments/{payment_id}/capture",
            body=capture2_body,
            extra_headers={"Idempotency-Key": str(uuid.uuid1())},
        ),
        options,
    )
    data = capture2.get("data") if isinstance(capture2.get("data"), dict) else {}
    paid = capture2.get("status") == 200 and str(data.get("status") or "").upper() == "PAID"
    return {
        "success": bool(paid),
        "error_message": "" if paid else _response_error(capture2) or "payment not paid",
        "payment_id": payment_id,
        "status": str(data.get("status") or ""),
        "steps": steps,
    }


def build_checkout_body(order: dict[str, Any], *, service_id: str | None = None) -> dict:
    amount = order_amount(order)
    merchant_id = order_merchant_id(order)
    service_id = service_id or order_service_id(order)
    return {
        "intent": order.get("payment_intent") or order.get("intent") or "EWALLET_QR",
        "order_pricing": {
            "payment_method_specific_pricing": [],
            "default_amount": {"amount": amount},
        },
        "selected_options_tokens": [],
        "merchant_id": merchant_id,
        "frontend_overrides": {
            "offline_methods": [],
            "payment_method_rollout": [],
            "exclude_paylater": False,
        },
        "service_id": service_id,
        "metadata": {"merchant_id": merchant_id},
    }


def build_promotions_evaluate_body(order: dict[str, Any], payment_token: str) -> dict:
    return {
        "order_id": str(order.get("payment_id") or ""),
        "payment_instructions": [
            {
                "token": payment_token,
                "amount": {"value": order_amount(order), "currency": order_currency(order)},
            }
        ],
        "transaction_type": "MERCHANT_TRANSACTION",
    }


def build_capture_body(
    order: dict[str, Any],
    payment_token: str,
    *,
    pin_token: str = "",
    challenge_id: str = "",
    client_id: str = "",
) -> dict:
    amount = order_amount(order)
    currency = order_currency(order)
    challenge = None
    if pin_token:
        challenge = {
            "action": None,
            "value": {"pin_token": pin_token},
            "type": "GOPAY_PIN_CHALLENGE",
            "metadata": {"challenge_id": challenge_id, "client_id": client_id},
        }
    return {
        "payment_instructions": [
            {
                "token": payment_token,
                "amount": {"value": amount, "currency": currency},
                "admin_fee_token": None,
            }
        ],
        "applied_promo_code": ["NO_PROMO_APPLIED"],
        "description": None,
        "payment_method": None,
        "channel_type": None,
        "additional_data": None,
        "challenge": challenge,
        "metadata": None,
        "checksum": None,
        "order_signature": None,
    }


def encode_tlv(items: list[tuple[str, str]]) -> str:
    return "".join(f"{tag}{len(value):02d}{value}" for tag, value in items if value is not None)


def crc16_ccitt(data: bytes) -> int:
    crc = 0xFFFF
    for byte in data:
        crc ^= byte << 8
        for _ in range(8):
            if crc & 0x8000:
                crc = ((crc << 1) ^ 0x1021) & 0xFFFF
            else:
                crc = (crc << 1) & 0xFFFF
    return crc


def build_additional_data_national(postal_code: str, additional: dict[str, str]) -> str:
    nested = encode_tlv(sorted((k, v) for k, v in additional.items() if v))
    parts = []
    if postal_code:
        parts.append(("61", postal_code))
    if nested:
        parts.append(("62", nested))
    return encode_tlv(parts)


def extract_payment_token(response: dict[str, Any]) -> str:
    for item in _payment_option_items(response):
        token = str(item.get("token") or "")
        if token:
            return token
    raise ValueError("payment option token missing")


def randomize_payment_option_token(token: str) -> str:
    data = decode_payment_option_token(token)
    data["payment_option_id"] = str(uuid.uuid4())
    encoded = json.dumps(data, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
    return base64.b64encode(encoded).decode("ascii")


def decode_payment_option_token(token: str) -> dict[str, Any]:
    token = str(token or "").strip()
    if not token:
        raise ValueError("payment option token missing")
    try:
        decoded = base64.urlsafe_b64decode(token + "=" * (-len(token) % 4)).decode("utf-8")
        data = json.loads(decoded)
    except Exception as exc:
        raise ValueError("payment option token is not decodable JSON") from exc
    if not isinstance(data, dict):
        raise ValueError("payment option token payload must be an object")
    if "payment_option_id" not in data:
        raise ValueError("payment option token missing payment_option_id")
    return data


def _payment_option_items(response: dict[str, Any]) -> list[dict[str, Any]]:
    candidates: list[Any] = []
    data = response.get("data")
    raw = response.get("raw") if isinstance(response.get("raw"), dict) else {}
    raw_data = raw.get("data")
    candidates.extend([data, raw_data])
    if isinstance(data, dict):
        candidates.extend([data.get("selected_options"), data.get("payment_options")])
    if isinstance(raw_data, dict):
        candidates.extend([raw_data.get("selected_options"), raw_data.get("payment_options")])

    items: list[dict[str, Any]] = []
    for candidate in candidates:
        if isinstance(candidate, list):
            items.extend(item for item in candidate if isinstance(item, dict))
    return items


def extract_challenge(response: dict[str, Any]) -> tuple[str, str]:
    data = response.get("data") if isinstance(response.get("data"), dict) else {}
    challenge = data.get("challenge") if isinstance(data.get("challenge"), dict) else {}
    value = ((challenge.get("action") or {}).get("value") or {}) if isinstance(challenge, dict) else {}
    metadata = challenge.get("metadata") or {}
    challenge_id = str(value.get("challenge_id") or metadata.get("challenge_id") or "")
    client_id = str(value.get("client_id") or metadata.get("client_id") or "")
    if not challenge_id or not client_id:
        raise ValueError(f"capture challenge missing: {_response_error(response) or response.get('raw')}")
    return challenge_id, client_id


def extract_pin_token(response: dict[str, Any]) -> str:
    data = response.get("data") if isinstance(response.get("data"), dict) else {}
    token = str(data.get("token") or data.get("pin_token") or "")
    if not token:
        raw = response.get("raw") if isinstance(response.get("raw"), dict) else {}
        token = str(raw.get("token") or "")
    if not token:
        raise ValueError("pin token missing")
    return token


def extract_payment_order(response: dict[str, Any]) -> dict[str, Any]:
    data = response.get("data") if isinstance(response.get("data"), dict) else {}
    if not data:
        raw = response.get("raw") if isinstance(response.get("raw"), dict) else {}
        data = raw.get("data") if isinstance(raw.get("data"), dict) else {}
    if not data:
        raise ValueError("payment detail missing order data")
    return data


def extract_linked_services(response: dict[str, Any]) -> list[dict[str, Any]]:
    data = response.get("data") if isinstance(response.get("data"), dict) else {}
    raw = response.get("raw") if isinstance(response.get("raw"), dict) else {}
    raw_data = raw.get("data") if isinstance(raw.get("data"), dict) else {}
    services = data.get("linked_services")
    if not isinstance(services, list):
        services = raw_data.get("linked_services")
    if not isinstance(services, list):
        return []
    return [item for item in services if isinstance(item, dict)]


def order_amount(order: dict[str, Any]) -> int:
    amount = order.get("amount") if isinstance(order.get("amount"), dict) else {}
    value = amount.get("value")
    if value is None:
        details = (((order.get("additional_data") or {}).get("aspiqr_information_v2") or {}).get("transaction_details") or {})
        nested = details.get("amount") if isinstance(details.get("amount"), dict) else {}
        value = nested.get("value")
    return int(float(value or 0))


def order_currency(order: dict[str, Any]) -> str:
    amount = order.get("amount") if isinstance(order.get("amount"), dict) else {}
    return str(amount.get("currency") or DEFAULT_CURRENCY)


def order_service_id(order: dict[str, Any]) -> str:
    widget = order.get("payment_widget_metadata") if isinstance(order.get("payment_widget_metadata"), dict) else {}
    metadata = order.get("metadata") if isinstance(order.get("metadata"), dict) else {}
    return str(order.get("service_id") or widget.get("service_id") or metadata.get("service_id") or DEFAULT_SERVICE_ID)


def order_merchant_id(order: dict[str, Any]) -> str:
    widget = order.get("payment_widget_metadata") if isinstance(order.get("payment_widget_metadata"), dict) else {}
    additional = order.get("additional_data") if isinstance(order.get("additional_data"), dict) else {}
    merchant = additional.get("merchant_information") if isinstance(additional.get("merchant_information"), dict) else {}
    root_merchant = order.get("merchant_information") if isinstance(order.get("merchant_information"), dict) else {}
    aspi = additional.get("aspiqr_information") if isinstance(additional.get("aspiqr_information"), dict) else {}
    return str(
        widget.get("merchant_id")
        or root_merchant.get("merchant_id")
        or root_merchant.get("id")
        or merchant.get("merchant_id")
        or merchant.get("id")
        or aspi.get("merchant_id")
        or ""
    )


def _call(steps: list[dict[str, Any]], label: str, fn, options: LinkPaymentOptions | LinkedAppUnlinkOptions) -> dict[str, Any]:
    response = fn()
    steps.append(step_result(label, response, options))
    return response


def step_result(label: str, response: dict[str, Any], options: LinkPaymentOptions | LinkedAppUnlinkOptions) -> dict[str, Any]:
    raw = response.get("raw")
    text = json_compact(raw) if isinstance(raw, (dict, list)) else str(raw or response.get("data") or "")
    return {
        "label": label,
        "status_code": int(response.get("status") or 0),
        "response_text": text[: max(int(options.body_limit or 1200), 1)],
        "error_message": _response_error(response),
    }


def _require_status(label: str, response: dict[str, Any], statuses: set[int]) -> None:
    status = int(response.get("status") or 0)
    if status not in statuses:
        raise ValueError(f"{label} failed: status={status} {_response_error(response)}")


def _response_error(response: dict[str, Any]) -> str:
    raw = response.get("raw") if isinstance(response.get("raw"), dict) else {}
    data = response.get("data") if isinstance(response.get("data"), dict) else {}
    errors = raw.get("errors") or data.get("errors") or []
    if isinstance(errors, list) and errors:
        first = errors[0] if isinstance(errors[0], dict) else {"message": str(errors[0])}
        code = str(first.get("code") or "")
        message = str(first.get("message") or first.get("message_title") or "")
        return f"{code} {message}".strip()
    if response.get("status") == 0:
        return str(data.get("error") or raw.get("error") or "")
    return ""


def json_compact(value: Any) -> str:
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"))


def parse_json_string(value: Any) -> Any:
    if isinstance(value, str):
        try:
            return json.loads(value)
        except Exception:
            return None
    return value
