FROM golang:1.24.1 AS build

WORKDIR /src

COPY go.mod go.sum .

RUN go mod download

COPY main.go VERSION .

RUN CGO_ENABLED=0 go build -o /litefolio/litefolio .

FROM alpine:3.17.0

WORKDIR /litefolio

COPY README.md LICENSE litefolio.yaml portfolio.yaml .
COPY ./assets ./assets
COPY ./styles ./styles
COPY ./templates ./templates

COPY --from=build /litefolio/litefolio .

CMD ["/litefolio/litefolio"]
