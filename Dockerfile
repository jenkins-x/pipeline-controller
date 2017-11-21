FROM scratch

ENTRYPOINT ["/pipeline-controller"]

COPY ./bin/pipeline-controller-linux-amd64 /pipeline-controller