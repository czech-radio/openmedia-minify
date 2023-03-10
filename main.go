package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/terminalstatic/go-xsd-validate"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// VERSION of openmedia-minify
const VERSION = "0.0.5"

func main() {

	log.Println(fmt.Sprintf("Openmedia-minify version: %s", VERSION))

	input := flag.String("i", "", "The input directory")
	output := flag.String("o", "", "The output directory")
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
		fmt.Println("openmedia-minify -i inputFolder -o output_folder")
	}

	err := ProcessFolder(*input, *output)
	if err != nil {
		log.Printf("Error processing folder %s: %s  ", *input, err)
	}

}

// ProcessFolder executes minify on each file in given folder outputs result to output folder
func ProcessFolder(input string, output string) error {

	files, err := ioutil.ReadDir(input)

	if err != nil {
		return err
	}

	// create tmp folder ///////////////////////////////////////////////////////////////////////////////////////////
	t := time.Now()
	tmp_folder := filepath.Join("/tmp", fmt.Sprintf("openmedia-minify-tmp_%s_%d", t.Format("20060102"),os.Getpid()))
	// check if file exist, if yes remove it
	if _, err := os.Stat(tmp_folder); err == nil {
		os.RemoveAll(tmp_folder)
	}

	err = os.Mkdir(tmp_folder, 0777)
	if err != nil {
		log.Printf("Creating tmp directory failed: %s\n", err.Error())
	}

	// minifying loop //////////////////////////////////////////////////////////////////////////
	var year, week int
	total := len(files)
	failed, passed := 0, 0
	var passedFiles []string
	var minifiedFilename string

	for index, file := range files {
		year, week, minifiedFilename, err = Minify(input, tmp_folder, file, index+1, total)
		if err != nil {
			log.Println("Minifier error: " + err.Error())
			failed++
		} else {
			passedFiles = append(passedFiles, minifiedFilename)
			passed++
		}
	}

	// zipping minified versions here /////////////////////////////////////////////////////////
	log.Printf("Zipping minified, no. of files: %d", passed)

	newFilename := fmt.Sprintf("%d_W%02d_MINIFIED", year, week) + ".zip"
	err = zipit(tmp_folder, filepath.Join(output, newFilename),false)
	if err != nil {
		log.Printf("Zipping minified results FAILED!: %s\n", err.Error())
		return errors.New("Failed to create zip archive: " + newFilename)
	}

	// zipping originals here /////////////////////////////////////////////////////////////////
	log.Printf("Zipping originals, no. of files: %d", total)
	newFilename = fmt.Sprintf("%d_W%02d_ORIGINAL", year, week) + ".zip"
	err = zipit(input, filepath.Join(output, newFilename),false)

	if err != nil {
		log.Printf("Zipping originals FAILED!: %s\n", err)
		return errors.New("Failed to create zip archive: " + newFilename)
	}

	// cleanup temporary files /////////////////////////////////////////////////////////////////

	// check if file exist, if yes remove it
	if _, err := os.Stat(tmp_folder); err == nil {
		os.RemoveAll(tmp_folder)
	}

	// final log

	log.Printf("Minifier finished, PASS/FAIL/TOTAL: %d/%d/%d", passed, failed, total)

	return nil

}

// ToXML helper function converts string to XML escaped string
func ToXML(input_string string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(input_string))
	return b.String()
}

// Minify reduces empty fields (whole lines) from XML file
func Minify(inpath string, outpath string, file os.FileInfo, index int, total int) (Year, Month int, MinifiedFilename string, Error error) {

	fext := filepath.Ext(file.Name())

	if file.IsDir() || fext != ".xml" || !strings.Contains(file.Name(), "RD") {
		return 0, 0, "n/a", errors.New("Skipping folder, non-XML file or non-RD file")
	}

	fptr, _ := os.Open(filepath.Join(inpath, file.Name()))
	scanner := bufio.NewScanner(transform.NewReader(fptr, unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder()))

	defer fptr.Close()

	var modded []string
	var counter int = 0
	var skipped int = 0
	for scanner.Scan() {
		line := fmt.Sprintln(scanner.Text())

		// skip non-filled lines, while holding structure ie. OM_FIELD, OM_OBJECT etc tags
		// skip duplicate document declaration in file (occasional) `<?xml...`

		if (strings.Contains(line, `IsEmpty = "yes"`) && strings.Contains(line, "OM_FIELD") && !strings.Contains(line, "OM_HEADER") && !strings.Contains(line, "OM_OBJECT") && !strings.Contains(line, "OM_RECORD")) ||
			(strings.Contains(line, `<?xml`) && counter != 0) {
			skipped++
			continue
		} else {

			// FIX encoding line to UTF-8
			if strings.Contains(line, "UTF-16") {
				line = strings.ReplaceAll(line, "UTF-16", "UTF-8")
			}

			counter++

			modded = append(modded, line)
		}

	}

	log.Println("Minifying: " + filepath.Join(inpath, file.Name()))
	log.Printf("Document minified from %d lines to %d lines, ratio: %f%%\n", skipped+counter, counter, ((float32)(counter)/((float32)(skipped)+(float32)(counter)))*100.0)

	// TODO: check validity of resulting XML file

	// new filename
	weekday, year, month, day, week := getDateFromFile(filepath.Join(inpath, file.Name()))
	split := strings.Split(file.Name(), "-")
	beginning := split[0] + "-" + split[1]

	// fix occassionally missing underscore
	if beginning[len(beginning)-1:] != "_" {
		beginning = beginning + "_"
	}

	new_filename := beginning + fmt.Sprintf("%s_W%02d_%04d_%02d_%02d", weekday, week, year, month, day)

	err := saveStringSliceToFile(filepath.Join(outpath, new_filename+".xml"), modded)
	if err != nil {
		log.Printf("Minifying FAILED! %d/%d\n", index, total)
		return 0, 0, "n/a", errors.New("Failed to save file " + filepath.Join(outpath, new_filename+".xml"))
	}

	log.Println("Validating source file: " + filepath.Join(inpath, file.Name()))
	err = IsValidXML(filepath.Join(inpath, file.Name()))
	if err != nil {
		log.Printf("Minifying FAILED! %d/%d\n", index, total)
		return 0, 0, "n/a", errors.New("Source file is not valid XML: " + filepath.Join(inpath, file.Name()) + " " + err.Error())
	}

	log.Println("Validating destination file: " + filepath.Join(outpath, new_filename+".xml"))
	err = IsValidXML(filepath.Join(outpath, new_filename+".xml"))
	if err != nil {
		err2 := markFileCorrupt(filepath.Join(outpath, new_filename+".xml"))
		if err2 != nil {
			log.Println("Error renaming file: " + filepath.Join(outpath, new_filename+".xml"))
		}
		log.Println("Minifying FAILED!")
		return 0, 0, "n/a", errors.New("Resulting file is not valid XML: " + filepath.Join(outpath, new_filename+".xml") + " " + err.Error())
	}

	log.Printf("Minifying PASSED! %d/%d\n", index, total)

	return year, week, fmt.Sprintf(new_filename + ".xml"), nil
}

// markFileCorrupt renames badly fromat file to *_MALFORMED filename
func markFileCorrupt(input string) error {

	dir, corruptFn := filepath.Split(input)
	corruptFn = strings.TrimSuffix(corruptFn, filepath.Ext(corruptFn))
	corruptFn = corruptFn + "_MALFORMED.xml"
	// check if file exist, if yes go on renaming it
	_, err := os.Stat(input)
	if err == nil {
		os.Rename(input, filepath.Join(dir, corruptFn))
	}

	return err

}


// openmedia-check function to get date from xml file
func getDateFromFile(filepath string) (Weekday string, Year, Month, Day, Week int) {

	var weekday string
	var year, month, day, week = 0, 0, 0, 0
	var scanner bufio.Scanner

	handle, err := os.Open(filepath)
	if err != nil {
		log.Fatal(err)
	}

	scanner = *bufio.NewScanner(transform.NewReader(handle, unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder()))

	for scanner.Scan() {
		var line = fmt.Sprintln(scanner.Text())

		if strings.Contains(line, `FieldID = "1004"`) {
			reg := regexp.MustCompile("([0-9][0-9][0-9][0-9]{1})([0-9]{2})([0-9]{2})(T)")
			res := reg.FindStringSubmatch(line)

			date, err := time.Parse("20060102", res[1]+res[2]+res[3])

			if err != nil {
				log.Fatal(err)
			}

			year, month, day = date.Year(), int(date.Month()), date.Day()
			year, week = date.ISOWeek()

			t, err := time.Parse(time.RFC3339, fmt.Sprintf("%04d-%02d-%02dT00:00:00Z", date.Year(), int(date.Month()), date.Day()))
			if err != nil {
				log.Fatal(err)
			}
			weekday = t.Weekday().String()
			break // Find first ocurrance!
		}
	}

	return weekday, year, month, day, week
}



// zipit is more compelete zipping function
func zipit(source, target string, needBaseDir bool) error {
	log.Println("Zipping: " + source + " to archive: " + target)
	
        zipfile, err := os.Create(target)
	if err != nil {
		return err
	}
	defer zipfile.Close()

	archive := zip.NewWriter(zipfile)
	defer archive.Close()

	info, err := os.Stat(source)
	if err != nil {
		return err
	}

	var baseDir string
	if info.IsDir() {
		baseDir = filepath.Base(source)
	}

	filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		if baseDir != "" {
			if needBaseDir {
				header.Name = filepath.Join(baseDir, strings.TrimPrefix(path, source))
			} else {
				path := strings.TrimPrefix(path, source)
				if len(path) > 0 && (path[0] == '/' || path[0] == '\\') {
					path = path[1:]
				}
				if len(path) == 0 {
					return nil
				}
				header.Name = path
			}
		}

		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}

		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(writer, file)
		return err
	})

	return err
}

// saveStringSliceToFile saves given string slice to a file
func saveStringSliceToFile(filename string, input []string) error {

	// check if file exist, if yes remove it
	if _, err := os.Stat(filename); err == nil {
		os.Remove(filename)
	}

	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

	if err != nil {
		log.Fatalf("failed creating file: %s", err)
	}

	datawriter := bufio.NewWriter(file)

	for _, data := range input {
		_, _ = datawriter.WriteString(data)
	}

	datawriter.Flush()
	file.Close()

	return err
}

// IsValidXML checks validity of an XML
func IsValidXML(inputFile string) error {
	xsdvalidate.Init()
	defer xsdvalidate.Cleanup()

	xmlFile, err := os.Open(inputFile)
	if err != nil {
		panic(err)
	}
	defer xmlFile.Close()

	// this line is quite memory hungry, can cause crashes
	inXml, err := ioutil.ReadAll(xmlFile)
	if err != nil {
		panic(err)
	}

	validator, err := xsdvalidate.NewXmlHandlerMem(inXml, xsdvalidate.ValidErrDefault)
	defer validator.Free()

	if err != nil {
		switch err.(type) {
		case xsdvalidate.ValidationError:
			log.Println(err)
			log.Printf("Error in line: %d\n", err.(xsdvalidate.ValidationError).Errors[0].Line)
			log.Println(err.(xsdvalidate.ValidationError).Errors[0].Message)
			return err
		default:
			//log.Println(err)
			return err
		}
	}

	return err
}
