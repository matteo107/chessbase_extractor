# Chessbase 2CBH extractor

Extracts a ChessBase database (`.2cbh` and its companion files) from a Windows
`SetupMega…exe` installer, so a database you've bought can be used with
ChessBase for Mac without running the Windows installer.

Tested with `SetupMega2025_2cbh_v2.exe` (Mega Database 2025).

## Why this project

ChessBase has finally released a Mac version of its famous program. But the
databases you bought are still delivered as Windows installers (`.exe`), which
won't run on a Mac.

If, like me, you moved from Windows to Mac and no longer have your ChessBase
databases, this tool gets them back: it extracts the database from the Windows
installer so you can import it into ChessBase for Mac.

## Getting started

1. Download the Mega Database you want to import from your
   [ChessBase downloads page](https://shop.chessbase.com/userspace/en/downloads).
2. Download `chessbase_extractor_<version>_macos_universal.zip` from the
   [latest release](https://github.com/matteo107/chessbase_extractor/releases/latest)
   and unzip it. The program runs on both Apple Silicon and Intel Macs.
3. Put the `chessbase_extractor` program and the downloaded `.exe` in the same
   folder, then open Terminal in that folder and run:

   ```bash
   xattr -d com.apple.quarantine chessbase_extractor
   ```

   ```bash
   ./chessbase_extractor
   ```

   The first command is needed only once. The program isn't signed by Apple,
   so macOS blocks it until you remove the "downloaded from the internet"
   mark. If it replies `No such xattr`, the mark wasn't there and you can
   carry on.
4. In ChessBase for Mac, choose **File → Open Database** and select the
   extracted `.2cbh` file from that folder (see
   [Importing into ChessBase for Mac](#importing-into-chessbase-for-mac)).

### Building from source

If you prefer, clone this project, copy the `.exe` into the project folder,
then build and run it yourself (requires Go 1.27+):

```bash
go build -o chessbase_extractor .
./chessbase_extractor [flags] [installer.exe]
```

### Verifying the download

Each release includes a `checksums.txt` file. To check the zip wasn't
corrupted or tampered with, run this in the folder holding both files:

```bash
shasum -a 256 -c checksums.txt
```

## How it works

The installer is two wrappers around a 7-Zip archive:

```
SetupMega2025_2cbh_v2.exe    NSIS installer (payload stored uncompressed)
└── Mega20252cbh.exe         7-Zip self-extracting stub
    └── 7z archive           the database files (LZMA2)
```

The NSIS layer stores its payload uncompressed, so the 7z archive sits byte
for byte inside the outer `.exe`. The program:

1. Scans the installer for the 7z signature (`7z\xBC\xAF\x27\x1C`).
2. Validates each match (header CRC, and the archive must fit inside the
   file) to rule out false positives.
3. Unpacks that archive straight from the installer, checking every file's
   CRC.

It does not need Wine, 7-Zip, or any other external tool.

## Requirements

- macOS (Apple Silicon or Intel)
- About 11 GB of free disk space for the Mega Database 2025 output
- Go 1.27+, only if you build from source

## Usage

From this folder, with the installer next to the program:

```bash
go run .
```

With no arguments it picks the only `.exe` in the current directory and
extracts into the current directory.

```bash
go build -o chessbase_extractor .
./chessbase_extractor [flags] [installer.exe]
```

| Flag | Default | Description |
|---|---|---|
| `-o dir` | `.` | Output directory |
| `-list` | off | List the embedded files without extracting |
| `-p password` | none | Archive password (only needed if the archive is encrypted; Mega 2025 isn't) |
| `-version` | off | Print the version and exit |

Examples:

```bash
./chessbase_extractor -list SetupMega2025_2cbh_v2.exe
```

```bash
./chessbase_extractor -o ~/ChessBase/Databases SetupMega2025_2cbh_v2.exe
```

Extracting Mega Database 2025 (10.9 GiB) takes about 3 minutes on an Apple
Silicon Mac.

## Output

| File | What it holds |
|---|---|
| `.2cbh` | Header/index: one record per game (players, event, date, result, ratings, ECO). Open this file in ChessBase. |
| `.2cbg` | Game moves |
| `.2cba` | Annotations: comments, variations, symbols, arrows and coloured squares |
| `.2lid`, `.2lgd`, `.2lcd` | Lookup and index tables (players, tournaments, sources, search indexes)\* |
| `.cko` | Opening key / classification index\* |
| `.cpo` | Search booster / position index\* |
| `.ini`, `.ico` | Database settings and icon |
| `….html/` | Player photos and other pictures |

\* The newer "2" format isn't publicly documented, so these roles are a best
guess.

## Importing into ChessBase for Mac

Keep **all** the extracted files together in one folder, then open
`Mega Database 2025.2cbh` in ChessBase. It finds the other files by name; if
any are missing, the database won't open or will show games without moves or
annotations.

Once the database opens correctly, you can delete the installer `.exe` to free
about 1.6 GB.

## Limitations

- It only handles installers that embed an uncompressed 7z archive (NSIS
  wrapper + 7-Zip SFX). Other installer types (Inno Setup, MSI, compressed
  NSIS) report `no embedded 7z archive found`.
- Paths that would escape the output directory are refused.

## Legal

Use this only on databases you have bought. It doesn't bypass any copy
protection: it unpacks the same files the Windows installer would.

This repository contains no ChessBase software, databases, installers or other
ChessBase content, and none should be added to it. Don't use this tool to
share or distribute databases.

### Non-affiliation and trademarks

This is an independent, unofficial project. It is not affiliated with,
endorsed by, sponsored by or connected to ChessBase GmbH in any way.

"ChessBase" and "Mega Database" are trademarks of ChessBase GmbH. Other names
may be trademarks of their respective owners. They are used here only to
describe what this tool works with.

This software is released under the [MIT License](LICENSE) and is provided
"as is", without warranty of any kind. You are
responsible for making sure your use complies with the licence terms of the
products you have bought.
