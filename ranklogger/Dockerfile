FROM golang:1.25.6-alpine AS builder
RUN apk add --no-cache gcc libgcc build-base
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -ldflags "-s -w" -o main main.go

FROM alpine:latest as runner
RUN apk add --no-cache libc6-compat
COPY --from=builder /app/main .
COPY --from=builder /app/config/config.yaml .
COPY --from=builder /app/openapi.yaml .
CMD ./main