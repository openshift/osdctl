FROM registry.svc.ci.openshift.org/openshift/release:golang-1.14

WORKDIR /src
COPY . .
RUN make build

FROM registry.access.redhat.com/ubi8/ubi-minimal:latest
LABEL io.openshift.managed.name="osd-utils-cli" \
      io.openshift.managed.description="OSD related command line utilities"

COPY --from=0 /src/bin/osd-utils-cli /bin/osd-utils-cli

ENTRYPOINT ["bin/osd-utils-cli"]
