FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-w -s" -o jpcorrect ./cmd/jpcorrect

# distroless/static ships CA certificates (JWKS is fetched over HTTPS), tzdata
# (for TZ below), and a nonroot user, with no shell or package manager.
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder /app/jpcorrect .

ENV TZ=Asia/Taipei

EXPOSE 8080

USER nonroot:nonroot

CMD ["/app/jpcorrect"]
