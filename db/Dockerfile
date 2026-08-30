FROM postgres:18-alpine

WORKDIR /app

COPY db/migrate.sh /app/migrate.sh
COPY db/migrations/investigations /app/migrations/investigations

RUN sed -i 's/\r$//' /app/migrate.sh && chmod +x /app/migrate.sh

ENTRYPOINT ["/app/migrate.sh"]
CMD ["/app/migrations/investigations/000.sql"]
