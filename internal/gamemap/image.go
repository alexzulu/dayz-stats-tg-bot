package gamemap

import (
	"image"
	"io"
)

// EncodeReader streams img through encode and hands back a reader of the encoded bytes. Encoding runs in a
// goroutine writing into an [io.Pipe], so the fully encoded image is never materialized - bytes are produced as
// the consumer reads them (disk write, network upload, ...). An encode failure surfaces from Read, not as [io.EOF].
//
// The returned reader MUST be closed. Closing it unwinds the encode goroutine even when the consumer stops
// reading early; skipping Close leaks that goroutine, which blocks forever on the pipe write.
//
// The encode argument matches the signature of [image/png.Encode] (so it can be passed as-is); wrap encoders
// that take options, like jpeg or gif, in a closure to bind them.
//
// Example:
//
//	img, err := DrawPoint(chernarusplus.Map, Point{PosX: 7500, PosY: 8800, TopLabel: "player"})
//	if err != nil {
//		return err
//	}
//
//	// bind the jpeg quality via a closure; png.Encode could be passed directly
//	r := EncodeReader(img, func(w io.Writer, m image.Image) error {
//		return jpeg.Encode(w, m, &jpeg.Options{Quality: 70})
//	})
//	defer func() { _ = r.Close() }()
//
//	// stream straight to disk without buffering the encoded image
//	f, err := os.Create("player.jpg")
//	if err != nil {
//		return err
//	}
//	defer func() { _ = f.Close() }()
//
//	_, err = io.Copy(f, r)
//	return err
func EncodeReader(img image.Image, encode func(w io.Writer, m image.Image) error) io.ReadCloser {
	pr, pw := io.Pipe()

	go func() {
		// CloseWithError(nil) is just Close, so a clean encode ends the stream with io.EOF
		_ = pw.CloseWithError(encode(pw, img))
	}()

	return pr
}
