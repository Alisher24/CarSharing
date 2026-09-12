FROM node:26.8-alpine@sha256:ef24c5053d50fdc3e4e56eb4e7ddb7861874ab0fdc797046ba897581deb8e868 AS dev
WORKDIR /app
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
CMD ["npm", "run", "dev"]

FROM dev AS build
RUN npm run build

FROM nginxinc/nginx-unprivileged:stable-alpine@sha256:442753882674b49ae2c1de83ed67896131c0777f56df5005e356e62bc3f7e7ce AS runtime
COPY infra/nginx.conf /etc/nginx/conf.d/default.conf
COPY infra/security-headers.conf /etc/nginx/security-headers.conf
COPY infra/error-json.conf /etc/nginx/error-json.conf
COPY --from=build /app/dist/ /usr/share/nginx/html/
EXPOSE 8080
