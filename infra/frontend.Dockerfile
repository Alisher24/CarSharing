FROM node:24.21.0-alpine@sha256:be80f76cf40ec8e42b9bec49f60a55e0660f30af58d3e5a25530785b30ea67e2 AS dev
WORKDIR /app
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
# The build stage runs the unit tests, and one of them holds the entry rules to the bounds the
# Credentials schema states: the contract belongs in the image beside the frontend, exactly as the
# repository keeps it beside the frontend directory, or that check would report a missing document.
COPY openapi/public.yaml /openapi/public.yaml
CMD ["npm", "run", "dev"]

FROM dev AS build
RUN npm run build

FROM nginxinc/nginx-unprivileged:stable-alpine@sha256:442753882674b49ae2c1de83ed67896131c0777f56df5005e356e62bc3f7e7ce AS runtime
COPY infra/nginx.conf /etc/nginx/conf.d/default.conf
COPY infra/security-headers.conf /etc/nginx/security-headers.conf
COPY infra/error-json.conf /etc/nginx/error-json.conf
COPY --from=build /app/dist/ /usr/share/nginx/html/
EXPOSE 8080
