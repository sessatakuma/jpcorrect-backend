FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -o jpcorrect ./cmd/jpcorrect

FROM golang:1.25-alpine

WORKDIR /app

COPY --from=builder /app/jpcorrect .

ENV TZ=Asia/Taipei

EXPOSE 8080

CMD ["/app/jpcorrect"]
