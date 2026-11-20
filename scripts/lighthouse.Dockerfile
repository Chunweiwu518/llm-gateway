# Lighthouse in a container: docker build -f scripts/lighthouse.Dockerfile -t lh scripts
FROM node:22-bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends chromium ca-certificates \
    && rm -rf /var/lib/apt/lists/*
RUN npm install -g lighthouse@12.8.2
ENV CHROME_PATH=/usr/bin/chromium
USER node
ENTRYPOINT ["lighthouse"]
