package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"golang.org/x/text/transform"
        "golang.org/x/text/encoding/unicode"
	"io/ioutil"
        "log"
	"os"
	"path/filepath"
        "strings"
        "bytes"
        "encoding/xml"
)

// VERSION of openmedia-minify
const VERSION = "0.0.1"

func main() {

	log.Println(fmt.Sprintf("Openmedia-minify version: %s", VERSION))

	input := flag.String("i", "", "The input directory")
	output := flag.String("o", "", "Output directory")
	flag.Parse()

	if *input == "" {
		log.Fatal("Please specify the input folder.")
		os.Exit(1)
	}

	if *output == "" {
		log.Fatal("Please specify the output folder.")
		os.Exit(1)
	}

	flag.Usage = func() {
		fmt.Println("Usage:")
		fmt.Println("openmedia-minify -i input_folder -o output_folder")
	}


        ProcessFolder(*input,*output)

}

// ProcessFolder executes minify on each file in given folder outputs result to output folder
func ProcessFolder(input string, output string) error {

	files, err := ioutil.ReadDir(input)

	if err != nil {
		return err
	}

	for _, file := range files {
		err := Minify(input,output,file)
		if err != nil {
                  log.Println("Warn: " + err.Error())
		}
	}

	return err

}

func ToXML(input_string string) string {
    var b bytes.Buffer
    xml.EscapeText(&b,[]byte(input_string))
    return b.String()
}

// Minify reduces empty fields (whole lines) from XML file
func Minify(inpath string, outpath string, file os.FileInfo) error {

	fext := filepath.Ext(file.Name())

	if file.IsDir() || fext != ".xml" {
		return errors.New("Skipping folder or non-xml file")
	}

	fptr, _ := os.Open(filepath.Join(inpath, file.Name()))
	scanner := bufio.NewScanner(transform.NewReader(fptr, unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder()))

	defer fptr.Close()

	var modded []string
	for scanner.Scan() {
		line := fmt.Sprintln(scanner.Text())

		if strings.Contains(line,`IsEmpty = "yes"`) || line == "" {
			continue
		} else {
                        modded = append(modded, line)
		}
	}
	
        // TODO: modded to a file here
	fmt.Println(modded)

        return nil
}

// 
func IsValidXML(input []string) bool {

 return true
}
