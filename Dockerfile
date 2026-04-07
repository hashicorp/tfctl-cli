ARG base_image=docker.artifactory.hashicorp.engineering/ubuntu:24.04
FROM ${base_image}

ARG PRODUCT_NAME
ARG PRODUCT_VERSION
# TARGETARCH and TARGETOS are set automatically when --platform is provided.
ARG TARGETOS TARGETARCH
ARG BUILD_DIRECTORY=dist/$TARGETOS/$TARGETARCH
ENV BIN_DIR=$BUILD_DIRECTORY

LABEL maintainer="HCP Terraform Support <tf-cloud@hashicorp.support>"
LABEL "com.hashicorp.${PRODUCT_NAME}.version"="${PRODUCT_VERSION}"
LABEL name=$PRODUCT_NAME
LABEL vendor="HashiCorp"
LABEL version=$PRODUCT_VERSION

RUN apt-get -y clean
RUN apt-get -y update && apt-get -y dist-upgrade

RUN apt-get -y install ca-certificates jq unzip curl

RUN groupadd --system tfcloud && useradd --system --create-home --gid tfcloud tfcloud

USER tfcloud
RUN mkdir /home/tfcloud/bin
COPY --chown=tfcloud $BIN_DIR/tfcloud /home/tfcloud/bin/

RUN mkdir -p /home/tfcloud/.config/tfcloud

ENV PATH=$PATH:/local/bin

WORKDIR /home/tfcloud

ENTRYPOINT ["/home/tfcloud/bin/tfcloud"]