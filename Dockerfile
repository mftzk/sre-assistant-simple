# Builder RUN steps have no network: modules come from vendor/, toolchain from the image.
FROM golang:1.26.2-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-mod=vendor GOTOOLCHAIN=local
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/sre-assistant ./cmd/sre-assistant

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/sre-assistant /sre-assistant
ENTRYPOINT ["/sre-assistant"]
