# BUILDER Image. Used to download all dependenices, etc
FROM golang:1.17.11-alpine3.16 as builder
# Changing root directory
WORKDIR /app
# Copy all files to root directory
COPY . .
# Collect and download dependances
RUN go mod download
# Building application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./main.go

# Result image
FROM alpine:3.13
# Volume directory
ARG APP_DATA_DIR=/app/storage
# Creating volume
RUN mkdir -p ${APP_DATA_DIR}
# Creating volume
VOLUME ["${APP_DATA_DIR}"]
# Copy executable file (builded application) from builder to root directory
COPY --from=builder /app/main .


# Builded application running
ENTRYPOINT ["/main"]