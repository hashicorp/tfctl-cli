# Copyright IBM Corp. 2026
# SPDX-License-Identifier: MPL-2.0

ARG base_image=docker.artifactory.hashicorp.engineering/ubuntu:24.04
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

RUN apt-get -y clean
RUN apt-get -y update && apt-get -y dist-upgrade

RUN apt-get -y install ca-certificates jq unzip curl

RUN groupadd --system tfctl && useradd --system --create-home --gid tfctl tfctl

USER tfctl
RUN mkdir /home/tfctl/bin
COPY --chown=tfctl $BIN_DIR/tfctl /home/tfctl/bin/

RUN mkdir -p /home/tfctl/.config/tfctl

ENV PATH=$PATH:/home/tfctl/bin

WORKDIR /home/tfctl

RUN tfctl --autocomplete-install

ENTRYPOINT ["/home/tfctl/bin/tfctl"]