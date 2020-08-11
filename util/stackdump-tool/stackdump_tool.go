package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
)

/*
This tool can take stackdumps generated from ziti-fabric inspect stackdump and separate them into one dump per file
So if the file contains 4 stackdumps, 2 for controller and 2 for a router with id 001, it will seprate them into
controller.0.dump
controller.1.dump
001.0.dump
001.1.dump
*/

func main() {
	inputFileName := os.Args[1]
	inputFile, err := os.Open(inputFileName)
	if err != nil {
		panic(err)
	}
	scanner := bufio.NewScanner(inputFile)

	var outputFile *os.File

	resultsRegEx, err := regexp.Compile("Results:.*")
	if err != nil {
		panic(err)
	}

	stackdumpIdRegEx, err := regexp.Compile(`(^.*)\.stackdump$`)
	if err != nil {
		panic(err)
	}

	counts := map[string]int{}

	for scanner.Scan() {
		if scanner.Err() != nil {
			panic(scanner.Err())
		}
		line := scanner.Text()
		if resultsRegEx.MatchString(line) {
			fmt.Printf("Line matches: %v\n", line)
		} else if stackdumpIdRegEx.MatchString(line) {
			stackDumpId := stackdumpIdRegEx.FindStringSubmatch(line)[1]
			count := counts[stackDumpId]
			counts[stackDumpId] = count + 1
			if outputFile != nil {
				if err = outputFile.Close(); err != nil {
					panic(err)
				}
			}
			outputFileName := fmt.Sprintf("%v.%v.dump", stackDumpId, count)
			fmt.Printf("New stackdump found: %v dumping to %v\n", stackDumpId, outputFileName)
			outputFile, err = os.Create(outputFileName)
			if err != nil {
				panic(err)
			}
		} else if outputFile != nil {
			_, err := outputFile.WriteString(line + "\n")
			if err != nil {
				panic(err)
			}
		}
	}
}
