# Nemo LMS Portal

Back-office web portal for the Nemo neobank platform (loan management, accounts,
compliance, scoring, reporting). Built with Vite, React, TypeScript, Tailwind and shadcn-ui.

Production: https://lms.athenafinance.cloud (deployed by `.github/workflows/deploy.yml`
as the `nemo-portal` image into the `lms` namespace on the Contabo k3s box).

## Local development

```sh
npm install
npm run dev          # http://localhost:8080
```

The dev server proxies `/proxy/<service>` to the locally running Go services
(see `vite.config.ts`). In production the same paths are served by `nginx.conf`.

## Build

```sh
npm run build        # outputs dist/
docker build -t nemo-portal .
```
