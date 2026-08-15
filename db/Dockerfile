FROM postgres:18-alpine

WORKDIR /app

COPY migrate.sh /app/migrate.sh
COPY migrations/investigations /app/migrations/investigations

RUN chmod +x /app/migrate.sh

ENTRYPOINT ["/app/migrate.sh"]
CMD ["/app/migrations/investigations/000.sql"]
