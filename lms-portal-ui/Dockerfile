# syntax=docker/dockerfile:1
# ─── Stage 1: Build ──────────────────────────────────────────────────────────
FROM node:20-alpine AS build
WORKDIR /app

# Install deps first (cached layer)
COPY package.json package-lock.json* ./
RUN npm ci --legacy-peer-deps

# Copy source and build
COPY . .
RUN npm run build

# ─── Stage 2: Serve ──────────────────────────────────────────────────────────
FROM nginx:1.27-alpine AS final
WORKDIR /usr/share/nginx/html

# Remove default nginx content
RUN rm -rf /usr/share/nginx/html/*

# Copy built assets
COPY --from=build /app/dist /usr/share/nginx/html

# Copy nginx config
COPY nginx.conf /etc/nginx/conf.d/default.conf

# Runtime config entrypoint: writes /usr/share/nginx/html/config.js from
# APP_MODE / DEMO_MODE / PUBLIC_VERIFY_BASE, then starts nginx.
COPY docker-entrypoint.sh /usr/local/bin/nemo-entrypoint.sh
RUN chmod +x /usr/local/bin/nemo-entrypoint.sh

# Remove the default nginx.conf listen on 80 that conflicts with our config on 3000
RUN sed -i '/listen\s*80/d' /etc/nginx/nginx.conf || true

EXPOSE 3000

ENV APP_MODE=portal DEMO_MODE=false PUBLIC_VERIFY_BASE=""

ENTRYPOINT ["/usr/local/bin/nemo-entrypoint.sh"]
CMD ["nginx", "-g", "daemon off;"]
