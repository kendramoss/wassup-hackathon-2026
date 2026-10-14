FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
# Precedent corpus: labels + facts sections of public CBP rulings (docs/DECISIONS.md D18).
COPY --from=build /src/data /data
WORKDIR /
COPY --from=build /server /server
ENTRYPOINT ["/server"]
