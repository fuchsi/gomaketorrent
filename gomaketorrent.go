/*
 * Copyright (c) 2017 Daniel Müller
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package main

import (
	"bufio"
	"crypto/sha1"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fuchsi/torrentfile"
	"github.com/pborman/getopt/v2"
)

const VERSION = "v0.1.0"

var helpFlag = getopt.BoolLong("help", 'h', "Show this help message and exit")
var versionFlag = getopt.BoolLong("version", 'V', "Print version and quit")
var announceOpt = getopt.ListLong("announce", 'a', "Announce URLs\nAt least one must be specified", "<url>[,<url>,...]")
var commentOpt = getopt.StringLong("comment", 'c', "", "Add a comment to the torrent file")
var pieceLengthOpt = getopt.UintLong("piece-length", 'l', 18, "Set the piece length to 2^n Bytes\ndefault is set to 18 = 2^18 Bytes = 256 KB")
var nameOpt = getopt.StringLong("name", 'n', "", "Set the name of the metainfo\ndefault is the basename of the target")
var outputOpt = getopt.StringLong("output", 'o', "", "Set the path and filename of the torrent file\ndefault is <name>.torrent")
var privateFlag = getopt.BoolLong("private", 'p', "Set the private flag")
var verboseFlag = getopt.BoolLong("verbose", 'v', "be verbose")
var debugFlag = getopt.BoolLong("debug", 'd', "debug output")

func main() {
	getopt.SetParameters("<target directory or filename>")

	// Parse the program arguments
	getopt.Parse()

	if *versionFlag {
		fmt.Println("maketorrent " + VERSION)
		return
	}
	if *helpFlag || getopt.NArgs() == 0 {
		getopt.Usage()
		return
	}

	if len(*announceOpt) == 0 {
		fmt.Fprintln(os.Stderr, "You need to specify at least one announce URL!")
		os.Exit(1)
	}

	if *pieceLengthOpt < 16 || *pieceLengthOpt > 25 {
		fmt.Fprintln(os.Stderr, "Invalid Piece Length!")
		fmt.Fprintln(os.Stderr, "The piece Length must be between 16 (64 KB) and 25 (32 MB)")
		os.Exit(1)
	}

	filename := getopt.Arg(0)
	finfo, err := os.Stat(filename)

	if os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "Target file or directory does not exist!")
		os.Exit(1)
	}

	tf := torrentfile.TorrentFile{}

	// Announce URLs
	tf.AnnounceUrl = (*announceOpt)[0]
	if len(*announceOpt) > 1 {
		al := make([]string, len(*announceOpt)-1)
		for i := 1; i < len(*announceOpt); i++ {
			al[(i - 1)] = (*announceOpt)[i]
		}
		tf.AnnounceList = al
	}

	// Comment
	if *commentOpt != "" {
		tf.Comment = *commentOpt
	}

	// Name
	if *nameOpt != "" {
		tf.Name = *nameOpt
	} else {
		tf.Name = filepath.Base(filename)
	}

	// Private flag
	if *privateFlag {
		tf.Private = true
	}

	// Piece length
	tf.PieceLength = uint64(math.Pow(float64(2), float64(*pieceLengthOpt)))

	tf.CreatedBy = "gomaketorrent " + VERSION
	tf.CreationDate = time.Now()
	tf.Encoding = "UTF-8"

	// Files
	if finfo.IsDir() { // Dir mode
		files, pieces := createFromDirectory(filename, tf.PieceLength)
		tf.Files = files
		tf.Pieces = pieces
	} else { // Single file mode
		tf.Files = make([]torrentfile.File, 1)
		file, pieces := createFromSingleFile(filename, tf.PieceLength)
		tf.Files[0] = file
		tf.Pieces = pieces
	}

	// Output
	var outfile string
	if *outputOpt != "" {
		outfile = *outputOpt
	} else {
		outfile = tf.Name + ".torrent"
	}
	_, err = os.Stat(outfile)
	if !os.IsNotExist(err) {
		overwrite := askForConfirmation("output file already exists. Overwrite?")
		if !overwrite {
			os.Exit(1)
		}
	}

	fp, err := os.Create(outfile)
	if err != nil {
		log.Fatal(err)
	}
	defer fp.Close()

	verboseOut("")
	verboseOutNoNl("Writing .torrent file...")
	writer := bufio.NewWriter(fp)
	writer.Write(tf.Encode())
	verboseOut("done")

	fmt.Println()
}

func askForConfirmation(s string) bool {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Printf("%s [y/n]: ", s)

		response, err := reader.ReadString('\n')
		if err != nil {
			log.Fatal(err)
		}

		response = strings.ToLower(strings.TrimSpace(response))

		if response == "y" || response == "yes" {
			return true
		} else if response == "n" || response == "no" {
			return false
		}
	}
}

func createFromSingleFile(filename string, pieceLength uint64) (torrentfile.File, [][torrentfile.PIECE_SIZE]byte) {
	finfo, _ := os.Stat(filename)
	file := torrentfile.File{Path: finfo.Name(), Length: uint64(finfo.Size())}
	nPieces := numPieces(file.Length, pieceLength)
	verboseOut(fmt.Sprintf("%d bytes in all", file.Length))
	verboseOut(fmt.Sprintf("That's %d pieces of %d bytes each", nPieces, pieceLength))

	pieces := make([][torrentfile.PIECE_SIZE]byte, nPieces)

	fp, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer fp.Close()
	reader := bufio.NewReader(fp)
	pieceIndex := 0

	for {
		buf := make([]byte, pieceLength)
		n, err := reader.Read(buf)
		if err != nil && err != io.EOF {
			log.Fatal(err)
		} else if err == io.EOF {
			break
		}
		if n < int(pieceLength) {
			pieceBuf := make([]byte, n)
			copy(pieceBuf, buf)
			buf = pieceBuf
		}
		pieces[pieceIndex] = sha1.Sum(buf)
		verboseOut(fmt.Sprintf("Hashed %d of %d pieces", pieceIndex+1, nPieces))
		pieceIndex++
	}

	return file, pieces
}

func createFromDirectory(filename string, pieceLength uint64) ([]torrentfile.File, [][torrentfile.PIECE_SIZE]byte) {
	filename = strings.TrimRight(filename, "/")
	files := collectFiles(filename)
	var totalSize uint64
	debug(fmt.Sprintf("Number of files: %d", len(files)))

	for i, f := range files {
		files[i].Path = strings.TrimPrefix(f.Path, filename+"/") // there must be a better way to alter the path
		totalSize += f.Length
		verboseOut(fmt.Sprintf("Adding %s (%d bytes)", files[i].Path, f.Length))
	}

	numPieces := numPieces(totalSize, pieceLength)
	verboseOut(fmt.Sprintf("%d bytes in all", totalSize))
	verboseOut(fmt.Sprintf("That's %d pieces of %d bytes each", numPieces, pieceLength))

	pieces := make([][torrentfile.PIECE_SIZE]byte, numPieces)

	bufLen := pieceLength
	pieceIndex := 0
	pieceBuf := make([]byte, pieceLength)
	off := uint64(0)
	c := make(chan piece)

	for _, f := range files {
		fp, err := os.Open(filename + "/" + f.Path)
		if err != nil {
			fp.Close()
			log.Fatal(err)
		}
		reader := bufio.NewReader(fp)

		for {
			buf := make([]byte, bufLen)
			n, err := reader.Read(buf)
			if err != nil && err != io.EOF {
				log.Fatal(err)
			} else if err == io.EOF {
				break
			}
			length := uint64(n)

			if length < bufLen { // got less bytes than pieceLen from file (reached EOF while reading)
				debug(fmt.Sprintf("p#%d Got only %d bytes", pieceIndex, length))
				bufLen = pieceLength - length
				debug(fmt.Sprintf("New bufLen = %d bytes", bufLen))
				debug(fmt.Sprintf("pieceBuf[%d] = 0x%X", off, pieceBuf[off]))
				copy(pieceBuf[off:], buf[:length]) // copy length bytes from buf to pieceBuf
				debug(fmt.Sprintf("copy(pieceBuf[%d:], buf[:%d])", off, length))
				off = length // set new offset for pieceBuf to length
				debug(fmt.Sprintf("New offset: %d", off))
			} else if off != 0 { // got the remaining bytes from the next file
				debug(fmt.Sprintf("p#%d Got all %d bytes, resetting offset and bufLen", pieceIndex, length))
				debug(fmt.Sprintf("pieceBuf[%d] = 0x%X", off, pieceBuf[off]))
				copy(pieceBuf[off:], buf) // copy remaining bytes from buf to pieceBuf
				debug(fmt.Sprintf("copy(pieceBuf[%d:], buf[:%d])", off, length))
				off = 0 // reset offset and bufLen
				bufLen = pieceLength
				debug(fmt.Sprintf("reset offset to 0 and bufLen to %d", pieceLength))
			} else { // normal operation, just copy buf to pieceBuf
				copy(pieceBuf, buf)
			}

			if off == 0 { // hash the piece if offset is zero
				go buildHash(piece{index: pieceIndex}, pieceBuf, c)
				//pieces[pieceIndex] = sha1.Sum(pieceBuf)
				//verboseOut(fmt.Sprintf("Hashed %d of %d pieces", pieceIndex+1, numPieces))
				pieceIndex++
			}
		}

		fp.Close()
	}

	// add remaining bytes from buffer
	if off != 0 {
		debug(fmt.Sprintf("Add %d remaining bytes to pieces list at index %d", off, pieceIndex))
		//pieces[pieceIndex] = sha1.Sum(pieceBuf[:off])
		go buildHash(piece{index: pieceIndex}, pieceBuf, c)
		//verboseOut(fmt.Sprintf("Hashed %d of %d pieces", pieceIndex+1, numPieces))
	}

	verboseOut("")
	for i := uint64(0); i < numPieces; i++ {
		p := <-c
		pieces[p.index] = p.hash
		verboseOutNoNl(fmt.Sprintf("Hashed %d of %d pieces\r", p.index+1, numPieces))
	}
	verboseOut("")

	return files, pieces
}

type piece struct {
	index int
	hash  [torrentfile.PIECE_SIZE]byte
}

func buildHash(p piece, data []byte, c chan piece) {
	p.hash = sha1.Sum(data)
	c <- p
}

func collectFiles(filename string) []torrentfile.File {
	var filelist []torrentfile.File
	files, err := ioutil.ReadDir(filename)
	if err != nil {
		log.Fatal(err)
	}
	for _, f := range files {
		if f.IsDir() {
			for _, inner := range collectFiles(filename + "/" + f.Name()) {
				filelist = append(filelist, inner)
			}
		} else {
			filelist = append(filelist, torrentfile.File{Length: uint64(f.Size()), Path: filename + "/" + f.Name()})
		}
	}

	return filelist
}

func numPieces(filesize, pieceLength uint64) uint64 {
	return uint64(math.Ceil(float64(filesize) / float64(pieceLength)))
}

func verboseOut(s string) {
	if *verboseFlag {
		fmt.Println(s)
	}
}

func verboseOutNoNl(s string) {
	if *verboseFlag {
		fmt.Print(s)
	}
}

func debug(v interface{}) {
	if *debugFlag {
		fmt.Print("debug: ")
		fmt.Println(v)
	}
}
