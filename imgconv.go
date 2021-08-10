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
    "os"
    "os/exec"
    "strconv"
    "strings"

    "github.com/rustyoz/svg"
)

var (
    err error
)

func ConvertWithAspect(data io.Reader, maxRes int, format string) (io.Reader, error) {
    data1, data := splitStream(data)
    w, h, err := getRes(data1)
    if err != nil { return data, err }
    w, h = scaleWithAspect(10, 10, maxRes)

    out, err := Convert(data, w, h, format)
    return out, err
}

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
    var convCmd    string
    var convArgs []string

    // Resolution cannot be 0 or less than -1, so return
    if w == 0 || h == 0 || w < -1 || h < -1 {
        err := errors.New("Invalid resolution; must either be -1 (native resolution) or above 0")
        return data, err
    }

    // Get original width and height
    var data2 io.Reader
    data2, data = splitStream(data)
    ow, oh, err := getRes(data2)
    if err != nil { return data, err }

    // Find a program capable of converting exporting the specified format
    convCmd, convArgs, err = getCmd("svg", format, ow, oh, w, h)
    if err != nil { return data, err }

    // Unset LD_LIBRARY_PATH before running command in case running inside an AppImage
    os.Unsetenv("LD_LIBRARY_PATH")

    cmd := exec.Command(convCmd, convArgs...)
    stdin, _  := cmd.StdinPipe()
    stdout, _ := cmd.StdoutPipe()
    stderr, _ := cmd.StderrPipe()

    go func() {
        defer stdin.Close()
        io.Copy(stdin, data)
    }()

    cmd.Start()

    // Return stderr if anything goes wrong
    // FIXME: Can't find a way to get Inkscape to shut up, there seems to be no
    // quiet mode, so using Inkscape will always return an error, even if
    // successful
    b := new(bytes.Buffer)
    b.ReadFrom(stderr)
    if b.String() != "" {
        err = errors.New(b.String())
    }

    return stdout, err
}

func getCmd(formatIn string, formatOut string, ow int, oh int, w int, h int) (string, []string, error) {
    var cmd    string
    var args []string

    // Sort programs by speed, convert is the slowest but has the widest
    // format support
    pref := []string{
        "rsvg-convert",
        "inkscape",
        "convert",
    }

    res := strconv.Itoa(w)+"x"+strconv.Itoa(h)
    dpi := calcDpi(ow, oh, w, h)

    progs := map[string][]string{
        "convert": {
            "-background", "none",
            "-density",    strconv.Itoa(dpi),
            "-resize",     res,
            "-",
            formatOut+":-",
        },

        "rsvg-convert": {
            "-w", strconv.Itoa(w),
            "-h", strconv.Itoa(h),
        },

        "inkscape": {
            "-p",
            "--export-type="+formatOut,
            "--export-filename", "-",
        },
    }

    // Inkscape doesn't have support for using -1 as regular resolution, so add
    // in width and height if the resolution asked for is 0 or greater
    if w >= 0 && h >=0 {
        progs["inkscape"] = append(progs["inkscape"], []string{
            "-w", strconv.Itoa(w),
            "-h", strconv.Itoa(h),
        }...)
    }

    fmtIn := map[string][]string{
        "convert": {
            "svg",
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
            "png",
        },
        "rsvg-convert": {
            "png",
        },
        "inkscape": {
            "png",
        },
    }

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

// Really just for ImageMagick currently, but possibly that other convert programs need it
func calcDpi(w1 int, h1 int, w2 int, h2 int) int {
        var maxRes int
        dpi := 96

        // Return default DPI if less than or equal to zero, otherwise the
        // program crashes from trying to divide by zero
        if w1 <= 0 || w2 <= 0 || h1 <=0 || h2 <= 0 {
            return dpi
        }

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

// getRes takes a datastream as input, returning the size of said image.
// Like the rest of this library, it also only supports SVGs
func getRes(data io.Reader) (int, int, error) {
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

    err = errors.New("Failed to get size information from image")
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
