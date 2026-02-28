FROM python:3.11-slim

WORKDIR /app

COPY yourtestsrv/ ./yourtestsrv/
COPY yourtestsrv.py ./
COPY config.json /etc/yourtestsrv/config.json

USER nobody:nogroup

EXPOSE 9000 9001 8080 1883

ENTRYPOINT ["python", "yourtestsrv.py"]
CMD ["serve-all", "--config", "/etc/yourtestsrv/config.json"]
