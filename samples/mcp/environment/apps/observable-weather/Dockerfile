FROM docker.io/library/python:3.12-alpine

WORKDIR /app
COPY server.py /app/server.py

EXPOSE 8080
ENTRYPOINT ["python", "/app/server.py"]
