# syntax=docker/dockerfile:1
# Dashboard image for docker compose: build the SPA, serve it with nginx, and
# proxy /api and /script.js to the API so everything is one origin (the
# session and CSRF cookies rely on that). Production uses a static host with
# the same proxy rules instead; see netlify.toml.
FROM node:22-alpine AS build
WORKDIR /web
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:1.29-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /web/dist /usr/share/nginx/html
