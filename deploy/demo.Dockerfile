# Fake HTTP service used as a demo target; build from the repository root.
FROM golang:1.26.2-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-mod=vendor GOTOOLCHAIN=local
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/demo ./cmd/demo

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/demo /demo
EXPOSE 8080
ENTRYPOINT ["/demo"]
