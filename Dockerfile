FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git
WORKDIR /build
COPY go.mod ./
COPY main.go ./
RUN CGO_ENABLED=0 go build -o /arch-docs main.go
RUN CGO_ENABLED=0 go install github.com/supermodeltools/graph2md@latest
RUN CGO_ENABLED=0 go install github.com/greynewell/pssg/cmd/pssg@v0.3.0

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /arch-docs /usr/local/bin/arch-docs
COPY --from=builder /go/bin/graph2md /usr/local/bin/graph2md
COPY --from=builder /go/bin/pssg /usr/local/bin/pssg
COPY templates/ /app/templates/
ENTRYPOINT ["/usr/local/bin/arch-docs"]
