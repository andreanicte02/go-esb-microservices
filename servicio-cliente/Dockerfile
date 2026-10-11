# Build from the repository root so the shared comun/ module is available:
#   docker build -f servicio-cliente/Dockerfile .
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY comun/ comun/
COPY servicio-cliente/go.mod servicio-cliente/go.sum servicio-cliente/
RUN cd servicio-cliente && go mod download
COPY servicio-cliente/ servicio-cliente/
RUN cd servicio-cliente && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app .

# No shell, no package manager, runs as a non-root user.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
