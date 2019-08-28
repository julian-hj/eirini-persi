FROM ubuntu

COPY binaries/eirini-ext /tmp/build/eirini-ext

ENV OPERATOR_WEBHOOK_HOST=34.83.207.215
ENV OPERATOR_WEBHOOK_PORT=4545
ENV NAMESPACE=cf-workloads

ENTRYPOINT ["/tmp/build/eirini-ext"]
