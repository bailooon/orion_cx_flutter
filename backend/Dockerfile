# Build stage -----------------------------------------------------------------
FROM golang:1.25-alpine AS build

WORKDIR /src

# Dependencies are copied first so that editing source code does not invalidate
# the module download layer.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO is off so the binary runs on a bare Alpine image with no libc surprises.
# The SQL migrations are embedded in the binary, so no extra files are shipped.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/orion ./cmd/orion

# Runtime stage ---------------------------------------------------------------
FROM alpine:3.20

# ca-certificates is required for the outbound HTTPS call to the Claude API;
# wget backs the container health check.
RUN apk add --no-cache ca-certificates wget && adduser -D -u 10001 orion

COPY --from=build /out/orion /usr/local/bin/orion

USER orion
ENV ORION_SERVICE=gateway
EXPOSE 8000 8010 8011 8012 8013

ENTRYPOINT ["/usr/local/bin/orion"]
