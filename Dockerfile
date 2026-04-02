FROM node:22-alpine AS frontend
WORKDIR /frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ .
RUN npm run build

FROM golang:1.25-alpine AS backend
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
COPY --from=frontend /frontend/dist ./web/dist
RUN go build -ldflags="-s -w" -o logdownloader ./cmd

FROM alpine:3.20
WORKDIR /app
COPY --from=backend /app/logdownloader .
EXPOSE 3000
CMD ["./logdownloader"]
