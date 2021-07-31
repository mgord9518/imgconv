// Copyright © 2021 Mathew Gordon <github.com/mgord9518>
//
// Permission  is hereby  granted,  free of charge,  to any person  obtaining a 
// copy of this software  and associated documentation files  (the “Software”),
// to   deal   in   the  Software   without  restriction,   including   without
// limitation the rights  to use, copy, modify, merge,   publish,   distribute,
// sublicense,  and/or sell copies of  the Software, and to  permit  persons to
// whom  the   Software  is  furnished  to  do  so,  subject  to  the following
// conditions:
// 
// The  above  copyright notice  and this permission notice  shall be  included
// in  all  copies  or substantial portions of the Software.
// 
// THE SOFTWARE IS PROVIDED “AS IS”, WITHOUT WARRANTY  OF ANY KIND,  EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED  TO  THE WARRANTIES  OF  MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE  AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS  OR COPYRIGHT  HOLDERS  BE  LIABLE FOR ANY CLAIM,  DAMAGES  OR OTHER
// LIABILITY, WHETHER IN  AN  ACTION OF CONTRACT, TORT  OR  OTHERWISE,  ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS
// IN THE SOFTWARE.

// The goal  of this library  is to regularize the syntax of commonly installed
// image editors and make them easier to interface  with in GoLang.  Created to
// assist in thumbnailing SVGs,  it currently only  supports svg2png conversion
// but I plan to add support for converting between most common formats.

package imgconv

import (
    "bytes"
    "errors"
    "io"
    "os/exec"
    "strconv"
    "strings"

    "github.com/rustyoz/svg"
)

var (
    err error

    // What each conversion program supports as input
    sprtIn = map[string][]string{
        "convert": {
            "png", "svg",
        },
        "rsvg-convert": {
            "svg",
        },
    }

    // And output
    sprtOut = map[string][]string{
        "convert": {
            "png",
        },
        "rsvg-convert": {
            "png",
        },
    }
)

func ConvertWithAspect(data io.Reader, maxRes int, format string) (io.Reader, error) {
    data1, data := splitStream(data)
    w, h, err := GetRes(data1)
    if err != nil { return nil, err }
    w, h = scaleWithAspect(10, 10, maxRes)

    out, err := Convert(data, w, h, format)
    return out, err
}

// Convert takes a reader (image) as input, returning a reader of the converted
// data in the format requested. If not successful, it will return the original
// image and an error.
func Convert(data io.Reader, w int, h int, format string) (io.Reader, error) {
    var convCmd    string
    var convArgs []string

    if !contains(sprtOut["convert"], format) {
        err := errors.New("Unsupported export format: "+format)
        return data, err
    }

    // Create a filename for our PNG icon
    if convCmd, err = exec.LookPath("rsvg-convert"); err == nil {
        convCmd  = "rsvg-convert"
        convArgs = []string{
            "-w", strconv.Itoa(w),
            "-h", strconv.Itoa(h),
        }
    } else if convCmd, err = exec.LookPath("inkscape"); err == nil {
        convCmd  = "inkscape"
        convArgs = []string{
            "-p",
            "--export-type="+format,
            "-w", strconv.Itoa(w),
            "-h", strconv.Itoa(h),
            "-o", "-",
        }
    } else if convCmd, err = exec.LookPath("convert"); err == nil {
        res := strconv.Itoa(w)+"x"+strconv.Itoa(h)

        // ImageMagick requires DPI, so we have to calculate it. If not
        // specified, ImageMagick will create a blurry image from many SVGs
        var data2 io.Reader
        data2, data = splitStream(data)
        ow, oh, _ := GetRes(data2)
        dpi := calcDpi(ow, oh, w, h)

        convArgs = []string{
            "-background", "none",
            "-density",    strconv.Itoa(dpi),
            "-resize",     res,
            "-",
            format+":-",
        }
    } else {
        err = errors.New("Failed to find supported image conversion software on this machine")
        return data, err
    }

    cmd := exec.Command(convCmd, convArgs...)
    stdin, err := cmd.StdinPipe()
    stdout, err := cmd.StdoutPipe()
    if err != nil { return nil, err }

    go func() {
        defer stdin.Close()
        io.Copy(stdin, data)
    }()

    cmd.Start()

    return stdout, err
}

// Takes image dimensions as input, returning those dimensions scaled while
// keeping the aspect ratio. Example: (10, 5, 512) returns (512, 256)
func scaleWithAspect(width int, height int, maxRes int) (int, int) {
    var h int
    var w int

    wratio := float32(width)/float32(height)
    hratio := float32(height)/float32(width)

    if wratio < hratio {
        wres := float32(maxRes) * wratio
        w = int(wres)
        h = maxRes
    } else {
        hres := float32(maxRes) * hratio
        h = int(hres)
        w = maxRes
    }

    return w, h
}

// Really just for ImageMagick currently, but possibly that other convert programs need it
func calcDpi(w1 int, h1 int, w2 int, h2 int) int {
        var maxRes int
        dpi := 96

        if w1 > h2 {
            maxRes = w1
        } else {
            maxRes = h1
        }

        n1 := w2 / maxRes
        n2 := h2 / maxRes

        if n1 < n2 {
            dpi = n1 * 96
        } else {
            dpi = n2 * 96
        }

        return dpi
}

// GetRes takes a datastream as input, returning the size of said image.
// Like the rest of this library, it also only supports SVGs
func GetRes(data io.Reader) (int, int, error) {
    // Load the SVG
    svg, err := svg.ParseSvgFromReader(data, "", 1)
    if err != nil { return 0, 0, err }

    w, _ := strconv.Atoi(svg.Width)
    h, _ := strconv.Atoi(svg.Height)

    // Set width and height based on viewbox if invalid
    if w < 1 || h < 1 {
        v := svg.ViewBox
        res := strings.Split(v, " ")

        // Format of ViewBox is: x1, y1, x2, y2
        x1, _ := strconv.ParseFloat(res[0], 32)
        x2, _ := strconv.ParseFloat(res[2], 32)
        y1, _ := strconv.ParseFloat(res[1], 32)
        y2, _ := strconv.ParseFloat(res[3], 32)

        // Subtract the 1st x/y val from the 2nd in case the first axis is negative, but this isn't super common
        w = int(float32(x2) - float32(x1))
        h = int(float32(y2) - float32(y1))
    }

    // Return if both width and height are valid
    if w > 0 && h > 0 {
        return w, h, nil
    }

    return 0, 0, err
}

func contains(slice []string, str string) bool {
    for _, i := range slice {
        if i == str { return true }
    }

    return false
}

// Splits a datastream in two
func splitStream(data io.Reader) (io.Reader, io.Reader) {
    buf := &bytes.Buffer{}
    tee := io.TeeReader(data, buf)

    return tee, buf
}