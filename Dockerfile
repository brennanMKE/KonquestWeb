# Parallax: one static binary with the web client embedded.
#   docker build -t parallax .
#   docker run -p 8080:8080 parallax
# Add System 1 computer players with, e.g.:
#   docker run -p 8080:8080 parallax -system1 http://host.docker.internal:8799/predict

FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.24-alpine AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -tags embed -trimpath -ldflags="-s -w" -o /parallax ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=server /parallax /parallax
EXPOSE 8080
ENTRYPOINT ["/parallax", "-addr", ":8080"]
