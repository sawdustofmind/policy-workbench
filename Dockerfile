FROM golang:1.25.6-alpine AS build
WORKDIR /src
COPY go.mod main.go TASK.md ./
COPY internal/ internal/
COPY web/ web/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/policy-workbench . \
    && mkdir -p /out/data

FROM scratch
WORKDIR /app
COPY --from=build /out/policy-workbench /app/policy-workbench
COPY --from=build --chown=10001:10001 /out/data/ /data/
USER 10001:10001
EXPOSE 8094
ENTRYPOINT ["/app/policy-workbench"]
CMD ["-addr", "0.0.0.0:8094", "-data", "/data/state-v4.json"]
