FROM golang:1.22-alpine AS build
WORKDIR /app

ENV CGO_ENABLED=0 GOOS=linux GOTOOLCHAIN=local

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -ldflags="-s -w" -trimpath -o /server

FROM scratch
COPY --from=build /server /server
EXPOSE 8080
ENTRYPOINT ["/server"]
