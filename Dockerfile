FROM node:alpine as builder

WORKDIR /clash-dashboard
Add . /clash-dashboard

RUN npm install -g pnpm

RUN pnpm install
RUN pnpm build

FROM metacubex/clash-meta:Alpha
COPY --from=builder /clash-dashboard/dist /root/.config/clash/dashboard
ADD dashboard /helper

WORKDIR /

ENTRYPOINT ["/helper"]