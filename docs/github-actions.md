# Building a get-cooked site in CI

## GitHub Pages

`get-cooked init` writes `.github/workflows/deploy.yml`. In the repository
settings, under **Pages**, set the source to **GitHub Actions**. Every push to
`main` then validates and deploys the site; pull requests only validate
(errors appear as annotations on the changed lines).

## The action

```yaml
- uses: RashKash103/get-cooked@v0
  with:
    version: latest       # a release like 0.3.0, or "latest"
    source: .             # site directory
    output: public        # where to write the site
    base-url: https://me.github.io/recipes/   # optional override of base_url
    args: --drafts        # optional extra build flags
```

The action downloads the release archive for the runner's OS and
architecture, verifies it against `checksums.txt`, and runs
`get-cooked build --format github`. Pin `version` for reproducible builds.

## Other CI systems or hosts

The CLI is a single static binary:

```sh
curl -fsSL -o gc.tgz https://github.com/RashKash103/get-cooked/releases/latest/download/get-cooked_linux_amd64.tar.gz
tar -xzf gc.tgz
./get-cooked validate --format github
./get-cooked build -o public --base-url "https://recipes.example.com/"
```

Upload `public/` to any static host (Netlify, Cloudflare Pages, S3, a plain
web server). The site uses relative-to-root links built from `base_url`, so
set it to the final public URL (the path matters for project pages such as
`https://user.github.io/repo/`).

## Validation only

```yaml
- run: get-cooked validate --format github
```

`validate` exits non-zero on errors. Warnings (untranslated strings, YAML
comments that swallowed text) are reported but don't fail the build.

## Serving with nginx or Docker

Any static file server works. For the site to install as an app (PWA) it
must be served over HTTPS (a TLS-terminating proxy is fine), and the service
worker and manifest should not be cached for long so updates arrive
promptly. A minimal nginx config:

```nginx
server {
    listen 80;
    root /usr/share/nginx/html;
    index index.html;
    absolute_redirect off;

    location / {
        try_files $uri $uri/ $uri.html =404;
    }
    # Fingerprinted assets never change.
    location /assets/ {
        expires 1y;
        add_header Cache-Control "public, immutable";
    }
    # Always revalidate pages, the service worker and the manifest.
    location ~* (\.html|/sw\.js|\.webmanifest)$ {
        add_header Cache-Control "no-cache";
    }
    error_page 404 /404.html;
}
```

A Docker image only needs to copy the built site:

```dockerfile
FROM nginx:alpine-slim
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY public/ /usr/share/nginx/html/
```

Build `public/` in CI with the action (or the CLI) before building the image.
