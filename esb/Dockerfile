# Build from the repository root so the shared comun/ module is available:
#   docker build -f esb/Dockerfile .
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY comun/ comun/
COPY esb/go.mod esb/go.sum esb/
RUN cd esb && go mod download
COPY esb/ esb/
RUN cd esb && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app .

# No shell, no package manager, runs as a non-root user.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
EXPOSE 8085
ENTRYPOINT ["/app"]
