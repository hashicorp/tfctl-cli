# Copyright IBM Corp. 2026
# SPDX-License-Identifier: MPL-2.0

ARG base_image=docker.mirror.hashicorp.services/alpine:3.21
FROM ${base_image}

ARG PRODUCT_NAME
ARG PRODUCT_VERSION
# TARGETARCH and TARGETOS are set automatically when --platform is provided.
ARG TARGETOS TARGETARCH
ARG BUILD_DIRECTORY=dist/$TARGETOS/$TARGETARCH
ENV BIN_DIR=$BUILD_DIRECTORY

LABEL maintainer="HCP Terraform Support <tfctl@hashicorp.support>"
LABEL "com.hashicorp.${PRODUCT_NAME}.version"="${PRODUCT_VERSION}"
LABEL name=$PRODUCT_NAME
LABEL vendor="HashiCorp"
LABEL version=$PRODUCT_VERSION

RUN set -eux && \
    apk add --no-cache ca-certificates

RUN addgroup tfctl && \
    adduser -S -G tfctl tfctl

USER tfctl
RUN mkdir /home/tfctl/dist
COPY --chown=tfctl $BIN_DIR/tfctl /home/tfctl/dist/

RUN mkdir -p /home/tfctl/.config/tfctl

ENV PATH=$PATH:/home/tfctl/dist

WORKDIR /home/tfctl

ENTRYPOINT ["/home/tfctl/dist/tfctl"]