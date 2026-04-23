# syntax=docker/dockerfile:1.7

# ---- build stage -----------------------------------------------------------
FROM golang:1.25-alpine AS builder

WORKDIR /src

# Cache deps separately from sources so unrelated code changes do not bust
# the module-download layer.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# -trimpath removes local GOPATH / module paths from the binary; the -s -w
# ldflags strip the symbol and DWARF tables. Together they yield a smaller,
# reproducible static binary.
RUN CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---- runtime stage ---------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/server /server

EXPOSE 3323
VOLUME ["/storage"]
USER nonroot:nonroot

ENTRYPOINT ["/server"]
