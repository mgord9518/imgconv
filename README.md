# imgconv
A GoLang library for converting images using existing software on the user's machine.

As of now only supports png to svg, but I have plans to support all image types in the supported programs (currently ImageMagick, Inkscape and rsvg-convert).

## API:
### Convert
```
func Convert(data io.Reader, w int, h int, format string)
```
Convert takes a reader (image) as input, returning a reader of the converted data in the format supplied. If not successful, it will return the original image and an error.

### ConvertWithAspect
ConvertWithAspect does the same thing as Convert, but takes only one dimension for size. The int represents the maximum length of the longer axis, while the shorter will be scaled proportionally.
```
func ConvertWithAspect(data io.Reader, maxRes int, format string) (io.Reader, error) {
```

### ConvertFile
```
ConvertFile(src string, dest string, w int, h int, format string) error {
```
ConvertFile takes a filepath, destination filepath, width, height and destination image format as input, returning a filepath of the converted image. If not successful, the file remains unchanged and no file will be supplied at 'dest'.

### ConvertFileWithAspect
```
func ConvertFileWithAspect(src string, dest string, maxRes int, format string) error {
```
ConvertFileWithAspect is a combination of ConvertWithAspect and ConvertFile.
