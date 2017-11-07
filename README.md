# gomaketorrent
Command line utility to create BitTorrent metainfo files

## Installation
```bash
$ dep ensure
$ go install
```

## Usage
```
Usage: gomaketorrent [-dhpvV] [-a <url>[,<url>,...]] [-c value] [-l value] [-n value] [-o value] <target directory or filename>
 -a, --announce=<url>[,<url>,...]
                   Announce URLs
                   At least one must be specified
 -c, --comment=value
                   Add a comment to the torrent file
 -d, --debug       debug output
 -h, --help        Show this help message and exit
 -l, --piece-length=value
                   Set the piece length to 2^n Bytes
                   default is set to 18 = 2^18 Bytes = 256 KB
 -n, --name=value  Set the name of the metainfo
                   default is the basename of the target
 -o, --output=value
                   Set the path and filename of the torrent file
                   default is <name>.torrent
 -p, --private     Set the private flag
 -v, --verbose     be verbose
 -V, --version     Print version and quit

```