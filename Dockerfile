FROM registry.ci.openshift.org/openshift/release:golang-1.14

WORKDIR /src
COPY . .
RUN make ci-build

FROM registry.access.redhat.com/ubi8/ubi-minimal:latest
LABEL io.openshift.managed.name="osdctl" \
      io.openshift.managed.description="OSD related command line utilities"

COPY --from=0 /src/dist/osdctl_linux_amd64/osdctl /bin/osdctl

ENTRYPOINT ["bin/osdctl"]
