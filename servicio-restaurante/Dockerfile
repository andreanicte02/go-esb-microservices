# Build from the repository root so the shared comun/ module is available:
#   docker build -f servicio-restaurante/Dockerfile .
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY comun/ comun/
COPY servicio-restaurante/go.mod servicio-restaurante/go.sum servicio-restaurante/
RUN cd servicio-restaurante && go mod download
COPY servicio-restaurante/ servicio-restaurante/
RUN cd servicio-restaurante && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app .

# No shell, no package manager, runs as a non-root user.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
EXPOSE 8081
ENTRYPOINT ["/app"]
