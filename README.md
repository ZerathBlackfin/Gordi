# Gordi

Self-hosted music tagger that files your library, one album at a time.

Drop albums in a folder. Gordi reads the files, finds the matching release on MusicBrainz and shows you every change it wants to make. You confirm, it rewrites the tags and moves the files into a named library.

Go backend, Svelte interface embedded in the binary, one container.

## Getting started

```sh
docker run -d \
  --name gordi \
  -p 7373:7373 \
  -v /path/to/incoming:/input \
  -v /path/to/library:/output \
  -v /path/to/config:/config \
  -e PUID=1000 \
  -e PGID=1000 \
  -e TZ=Europe/Paris \
  zerathblackfin/gordi:latest
```

Open `http://<server>:7373`, where `<server>` is the machine you started it on.

Or with docker-compose:

```yaml
services:
  gordi:
    image: zerathblackfin/gordi:latest
    container_name: gordi
    ports:
      - 7373:7373
    volumes:
      - /path/to/incoming:/input
      - /path/to/library:/output
      - /path/to/config:/config
    environment:
      PUID: "1000"
      PGID: "1000"
      TZ: "Europe/Paris"
    restart: unless-stopped
```

The same image is on GHCR, as `ghcr.io/zerathblackfin/gordi`, if you prefer it.

To build it yourself instead:

```sh
git clone https://github.com/ZerathBlackfin/Gordi.git
cd Gordi
cp .env.example .env   # set your two folders
docker compose up --build
```

## Volumes

| Path | What goes there |
| --- | --- |
| `/input` | the folder to sort through |
| `/output` | your library |
| `/config` | the database |

If you lose `/config`, Gordi rebuilds the queue from the input folder and fetches the MusicBrainz answers again. You only lose your settings.

## User and group

Gordi writes into your library, so the files should belong to you and not to root. Set `PUID` and `PGID` to your own user:

```sh
id your-user
# uid=1000(you) gid=1000(you)
```

On Synology the pair is usually `1026` and `100`. Left unset, Gordi writes as root, which most NAS handle badly.

## Settings

Everything comes from the environment. The naming patterns and a few preferences can also be changed later on `/settings`.

| Variable | Default | What it does |
| --- | --- | --- |
| `GORDI_INPUT` | `./media/inbox` | folder to sort through |
| `GORDI_OUTPUT` | `./media/library` | destination |
| `GORDI_MODE` | `move` | `move` or `copy` (keeps the original) |
| `GORDI_PORT` | `7373` | host port, used by docker-compose only |
| `GORDI_ADDR` | `:7373` | address Gordi listens on inside the container |
| `GORDI_DB` | `/config/gordi.bolt` | where the database lives |
| `GORDI_LANG` | `en` | `en` or `fr`, interface and messages |
| `GORDI_SCAN_EVERY` | `60` | seconds between scans |
| `GORDI_CONTACT` | project URL | contact URL sent to MusicBrainz |
| `GORDI_PREFETCH_EVERY` | `3` | seconds between prefetches, `0` disables |
| `GORDI_PATTERN` | see below | tree for an ordinary album |
| `GORDI_PATTERN_MULTI` | see below | tree for a multi-disc album |
| `PUID` / `PGID` | `0` | who the files belong to (see User and group) |
| `TZ` | `Europe/Paris` | timezone |

## Naming

Two patterns, picked automatically:

```
ordinary album   {artist}/{album} ({year})/{track} - {title}
several discs    {artist}/{album} ({year})/CD{disc:0}/{track} - {title}
```

Fields: `{artist}` `{album}` `{year}` `{track}` `{title}` `{disc}` `{format}`. A slash makes a folder, the extension is added automatically, and `{track:000}` sets the padding. You can change them under `/settings`.

## Before it writes anything

Filing moves your files, so:

- nothing is written until you have seen the full list of destinations;
- an existing file is never overwritten. Filing stops instead;
- copies go to a temporary name then get renamed, so no half-written file ever appears in your library;
- in move mode the originals are deleted only once everything else worked. If anything fails, what was just written is removed and the originals stay put;
- files are matched to tracks by number, then by title, and by order only as a last resort. Rows matched that way are flagged.

## What the scan picks up

Any folder layout works, at any depth, as long as there is **one album per folder containing audio**. `inbox/rock/2015/Album/*.flac` works like `inbox/Album/*.flac`.

Multi-disc albums are put back together, whether the discs are nested (`Album/CD1`) or side by side (`Album CD1`). A folder holding both tracks and a `CD2` subfolder is too ambiguous, so it is left alone.

Hidden folders are skipped. Cover art, `.torrent` and `.nfo` files are listed as extras and left where they are.

## Updating

```sh
docker compose pull && docker compose up -d
```

Or pull the new image from your NAS interface and recreate the container. The queue and the cache are rebuilt on the next scan, so an update costs you a rescan and nothing else.

## License

[AGPL-3.0](LICENSE). Use it, change it, run it. If you publish a modified version, or run one that other people use over a network, the source has to stay open.
