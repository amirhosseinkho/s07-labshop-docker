# LabShop API image.
FROM golang:1.22-alpine

WORKDIR /app
COPY . .
RUN go build -o api .

# defaults for local runs
ENV PORT=8080
ENV DB_PASSWORD=labshop-dev-2024

EXPOSE 8080
ENTRYPOINT ["./start.sh"]
