# Wolfi base, pinned by digest. apk provides the runtime tools Alpine had;
# nginx-stable matches the stable branch the UBI image used. certbot has no
# Wolfi package, so it is installed with pip into the Wolfi Python.
FROM cgr.dev/chainguard/wolfi-base@sha256:e9b8c3739e5fb5fc778843a2ce74f8620588c9ddc9c44918cead527abcb0b628

RUN apk add --no-cache \
      bash curl less ca-certificates-bundle git tzdata zip gettext \
      nginx-stable cronie supervisor python3 py3-pip && \
    pip3 install --no-cache-dir certbot==5.10.0 certbot-nginx==5.10.0 && \
    addgroup -S nginx && \
    adduser -D -S -h /var/lib/nginx -G nginx nginx && \
    mkdir -p /run/nginx /etc/nginx/http.d /var/log/nginx /var/spool/cron && \
    chown -R nginx:nginx /var/lib/nginx/tmp /var/lib/nginx/logs

STOPSIGNAL SIGINT
EXPOSE 80
EXPOSE 443
ENTRYPOINT ["/usr/bin/supervisord","-c","/etc/supervisord.conf"]

WORKDIR /usr/share/nginx/html
# copy configuration files
COPY docker/default.conf /etc/nginx/http.d/default.conf
COPY docker/nginx.conf /etc/nginx/nginx.conf
COPY docker/init_cert.sh /usr/local/init_cert.sh
COPY docker/init_react_envs.sh /usr/local/init_react_envs.sh
RUN chmod +x /usr/local/init_cert.sh
RUN chmod +x /usr/local/init_react_envs.sh

# configure supervisor
COPY docker/supervisord.conf /etc/supervisord.conf
# copy build files
COPY out/ /usr/share/nginx/html/
