FROM golang:1.11-alpine as builder
RUN apk add --no-cache git
ENV GOOS=linux
ENV CGO_ENABLED=0
ENV GO111MODULE=on
COPY . /src
WORKDIR /src
RUN rm -f go.sum
RUN go get
RUN go test ./...
RUN go build -a -installsuffix cgo -o tobac

FROM alpine:3.5
MAINTAINER Kim Tore Jensen <kimtjen@gmail.com>
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /src/tobac /app/tobac
CMD ["/app/tobac"]
