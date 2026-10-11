# Build from the repository root so the shared comun/ module is available:
#   docker build -f servicio-repartidor/Dockerfile .
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY comun/ comun/
COPY servicio-repartidor/go.mod servicio-repartidor/go.sum servicio-repartidor/
RUN cd servicio-repartidor && go mod download
COPY servicio-repartidor/ servicio-repartidor/
RUN cd servicio-repartidor && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app .

# No shell, no package manager, runs as a non-root user.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
EXPOSE 8082
ENTRYPOINT ["/app"]
