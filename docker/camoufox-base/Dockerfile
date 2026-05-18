FROM docker.m.daocloud.io/library/python:3.12-slim-bookworm

WORKDIR /app

RUN sed -i 's/deb.debian.org/mirrors.ustc.edu.cn/g' /etc/apt/sources.list.d/debian.sources && \
    apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        fonts-noto-cjk \
        git \
        libasound2 \
        libatk-bridge2.0-0 \
        libatk1.0-0 \
        libcairo2 \
        libcups2 \
        libdbus-glib-1-2 \
        libdbus-1-3 \
        libdrm2 \
        libgbm1 \
        libglib2.0-0 \
        libgtk-3-0 \
        libnspr4 \
        libnss3 \
        libpango-1.0-0 \
        libx11-6 \
        libx11-xcb1 \
        libxcb1 \
        libxcomposite1 \
        libxdamage1 \
        libxext6 \
        libxfixes3 \
        libxkbcommon0 \
        libxrandr2 \
        tzdata \
        wget \
        xdg-utils \
        xauth \
        xvfb \
    && rm -rf /var/lib/apt/lists/*

COPY register_requirements.txt /tmp/nb-register-requirements.txt
ARG CAMOUFOX_FETCH_PROXY=http://host.docker.internal:10809
RUN pip config set global.index-url https://pypi.tuna.tsinghua.edu.cn/simple \
    && pip install --no-cache-dir -r /tmp/nb-register-requirements.txt \
    && HTTP_PROXY=${CAMOUFOX_FETCH_PROXY} HTTPS_PROXY=${CAMOUFOX_FETCH_PROXY} python -m camoufox fetch \
    && rm /tmp/nb-register-requirements.txt

RUN apt-get update && apt-get install -y --no-install-recommends \
        libxt6 \
    && rm -rf /var/lib/apt/lists/*
