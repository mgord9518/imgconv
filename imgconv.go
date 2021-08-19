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

// Currently  only  supports  Linux  (and  maybe macOS if it has ImageMagick?),
// supporting  Windows  will  be very limited, because to my knowledge it comes
// with no software capable of converting images on the CLI by default

package imgconv

import (
    "bytes"
    "errors"
    "io"
    "os"
    "os/exec"
    "strconv"
    "strings"

    svg  "github.com/rustyoz/svg"
    mime "github.com/gabriel-vasile/mimetype"
)

var (
    err error
)

// Does the same thing as Convert, but only uses one dimension as input, it
// keeps the aspect ratio, using the input value as the maximum width or height
// of the final image
func ConvertWithAspect(data io.Reader, maxRes int, format string) (io.Reader, error) {
    var w, h int

    // This monstrosity is to split the original datastream into 3. If there
    // is a better way to do this I'm all ears
    buf := &bytes.Buffer{}
    tee := io.TeeReader(data, buf)
    n2 := io.MultiReader(buf, data)
    buf2 := &bytes.Buffer{}
    tee2 := io.TeeReader(n2, buf2)
    n := io.MultiReader(buf2, n2)

    mimetype, err := GetType(tee)
    if mimetype == "svg" {
        ow, oh, err := getSvgRes(tee2)
        if err != nil { return n, err }
        w, h = scaleWithAspect(ow, oh, maxRes)
    } else {
        w, h = maxRes, maxRes
    }

    out, err := Convert(n, w, h, format)
    return out, err
}

// Combination of ConvertFile and ConvertWithAspect
func ConvertFileWithAspect(src string, dest string, maxRes int, format string) error {
    in, err := os.Open(src)
    if err != nil { return err }

    out, err := ConvertWithAspect(in, maxRes, format)
    if err != nil { return err }

    file, err := os.Create(dest)
    if err != nil { return err }

    _, err = io.Copy(file, out)
    file.Close()
    if err != nil {
        os.Remove(dest)
        return err
    }

    return nil
}

// Convert takes a reader (image) as input, returning a reader of the converted
// data in the format requested. If not successful, it will return the original
// image and an error.
func Convert(data io.Reader, w int, h int, format string) (io.Reader, error) {
    // Resolution cannot be 0 or less than -1, so return
    if w == 0 || h == 0 || w < -1 || h < -1 {
        err := errors.New("Invalid resolution; must either be -1 (native resolution) or above 0")
        return data, err
    }

    buf := &bytes.Buffer{}
    tee := io.TeeReader(data, buf)
    n := io.MultiReader(buf, data)

    mimetype, err := GetType(tee)
    if err != nil { return n, err }

    // Find a program capable of converting exporting the specified format
    convCmd, convArgs, err := getCmd(mimetype, format, w, h)
    if err != nil { return n, err }

    // Unset LD_LIBRARY_PATH before running command in case running inside an AppImage
    os.Unsetenv("LD_LIBRARY_PATH")

    var b bytes.Buffer

    cmd := exec.Command(convCmd, convArgs...)
    stdin, _  := cmd.StdinPipe()
    cmd.Stderr = &b

    go func() {
        defer stdin.Close()
        io.Copy(stdin, n)
    }()

    // Run the command and buffer the output
    byteSlice, err := cmd.Output()
    stdout := bytes.NewReader(byteSlice)

    // If the command exits non-zero status, return stderr as the error message
    if err != nil {
        err = errors.New(b.String())
    }

    return stdout, err
}

// getCmd attempts to find a suitable command to convert to the requested
// format from the start format
func getCmd(formatIn string, formatOut string, w int, h int) (string, []string, error) {
    var cmd    string
    var args []string

    //TODO: Create a struct and use it instead of all these cluttered maps
    // Sort programs by speed, convert is the slowest but has the widest
    // format support
    pref := []string{
        "rsvg-convert",
        "inkscape",
        "convert",
    }

    res := strconv.Itoa(w)+"x"+strconv.Itoa(h)

    progs := map[string][]string{
        "convert": {
            "-background", "none",
            "-resize",     res,
            "-",
            formatOut+":-",
        },

        "rsvg-convert": {
            "-w", strconv.Itoa(w),
            "-h", strconv.Itoa(h),
            "-f", formatOut,
        },

        "inkscape": {
            "-p",
            "--export-type="+formatOut,
            "--export-filename", "-",
        },
    }

    // Inkscape doesn't have support for using -1 as regular resolution, so add
    // in width and height if the resolution asked for is 0 or greater
    if w > 0 && h > 0 {
        progs["inkscape"] = append(progs["inkscape"], []string{
            "-w", strconv.Itoa(w),
            "-h", strconv.Itoa(h),
        }...)
    }

    // The DPI for convert is set to 3072 because it's the ideal DPI for
    // converting a 16x16 SVG image to 512x512, which feels like a reasonable
    // medium, especially because ImageMagick is less than ideal for converting
    // SVGs anyway. If w and h set to -1, the density will not be changed
    if formatIn == "svg" && w > 0 && h > 0 {
        progs["convert"] = append([]string{
            "-density",    "3072",
        }, progs["convert"]...)
    }

    // Formats these programs support taking as input
    // This is partially limited by the formats the mimetype library supports
    fmtIn := map[string][]string{
        "convert": {
            "svg", "png", "xpm", "jxl", "jp2", "jpf",
            "jpg", "gif", "webp","bmp", "ico", "bpg",
            "dwg", "icns","heic","heif","hdr", "xcf",
            "pat", "gbr",
        },
        "rsvg-convert": {
            "svg",
        },
        "inkscape": {
            "svg",
        },
    }

    // And output
    fmtOut := map[string][]string{
        "convert": {
            "png", "xpm", "jxl", "jp2", "jpf", "gbr",
            "jpg", "gif", "webp","bmp", "ico", "bpg",
            "dwg", "icns","heic","heif","hdr", "xcf",
            "pat",
        },
        "rsvg-convert": {
            "png", "pdf", "ps",  "eps", "svg", "xml",
        },
        "inkscape": {
            "png", "pdf", "ps",  "eps", "svg",
        },
    }

    // Find a suitable command to convert from the input format to the output
    for _, i := range pref {
        if cmd, err = exec.LookPath(i); err == nil &&
        contains(fmtIn[i], formatIn) && contains(fmtOut[i], formatOut) {
            args = progs[i]
            return cmd, args, nil
        } else {
            continue
        }
    }

    err := errors.New("Failed to find a suitable image conversion program on "+
                      "this machine to convert "+formatIn+" to "+formatOut)
    return "", []string{}, err
}

// ConvertFile does the same thing as Convert, just directly to a file
func ConvertFile(src string, dest string, w int, h int, format string) error {
    in, err := os.Open(src)
    if err != nil { return err }

    out, err := Convert(in, w, h, format)
    if err != nil { return err }

    file, err := os.Create(dest)
    if err != nil { return err }

    _, err = io.Copy(file, out)
    file.Close()
    if err != nil {
        os.Remove(dest)
        return err
    }

    return nil
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

// GetType returns the common file extension of the image presented
func GetType(data io.Reader) (string, error) {
    m, err := mime.DetectReader(data)
    if err != nil { return "", err }
    s := strings.Split(m.String(), "/")

    if s[0] != "image" {
        err := errors.New("Data magic wasn't detected as an image format")
        return "", err
    } else {
        return strings.Replace(m.Extension(), ".", "", 1), nil
    }
}

// getSvgRes takes a datastream as input, returning the size of said image.
// Like the rest of this library, it also only supports SVGs
func getSvgRes(data io.Reader) (int, int, error) {
    // Load the SVG
    svg, err := svg.ParseSvgFromReader(data, "", 1)
    if err != nil { return -1, -1, err }

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

    err = errors.New("Failed to get size information from image")
    return -1, -1, err
}

func contains(slice []string, str string) bool {
    for _, i := range slice {
        if i == str { return true }
    }

    return false
}
