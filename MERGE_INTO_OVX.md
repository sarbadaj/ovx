> Historical package-import instructions. This checkout already contains the implementation; use [README.md](README.md) for current setup.

# Merge this MVP into `sarbadaj/ovx`

The public OVX repository currently contains the architecture/documentation layer. This package adds the first runnable implementation without replacing `README.md` or `docs/`.

## 1. Clone OVX

```bash
git clone https://github.com/sarbadaj/ovx.git
cd ovx
git checkout -b feat/mvp-runtime
```

## 2. Copy this package into the repository root

From the directory containing `ovx-mvp-complete`:

```bash
rsync -av \
  --exclude README.md \
  /absolute/path/to/ovx-mvp-complete/ \
  /absolute/path/to/ovx/
```

This adds:

```text
apps/api
services/runtime
docker
examples
bin
docker-compose.yml
Makefile
.env.example
README_MVP.md
```

It does not replace the existing architecture docs.

## 3. Remove repository noise before committing

```bash
find . -name .DS_Store -delete
```

## 4. Set up and run

Continue with `README_MVP.md`.

## 5. Commit

```bash
git add .
git status
git commit -m "feat: implement OVX coding-agent MVP"
```
