package main

// The product executable derives its Windows icon from the same canonical PNG
// embedded by aram-frontend for the running window. The generated resource
// object is ignored by Git and linked automatically into windows/amd64 builds.

//go:generate go run ../aram-iconpack -source ../../../aram-frontend/frontend/assets/icon.png -ico ../../build/icons/aram.ico
//go:generate go run github.com/akavel/rsrc@v0.10.2 -ico ../../build/icons/aram.ico -arch amd64 -o rsrc_windows_amd64.syso
