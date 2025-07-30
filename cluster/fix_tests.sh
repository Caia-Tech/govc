#!/bin/bash

# Fix all NewNode calls in test files
sed -i '' 's/NewNode("\([^"]*\)", "\([^"]*\)", \([0-9]*\))/createTestNode("\1", "\2", \3)/g' cluster/*_test.go

# Fix NewNode calls with fmt.Sprintf
sed -i '' 's/NewNode(fmt\.Sprintf/createTestNode(fmt.Sprintf/g' cluster/*_test.go