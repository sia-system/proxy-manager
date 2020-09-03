FROM debian:buster-slim
WORKDIR /

ADD ./proxy-manager /

RUN adduser -u 1000 --disabled-password --no-create-home --gecos "" app_user

RUN chmod +x ./proxy-manager
RUN chown 1000 ./proxy-manager

USER app_user

CMD ["./proxy-manager"]