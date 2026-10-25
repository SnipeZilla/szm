module github.com/SnipeZilla/szm

go 1.24.0

toolchain go1.24.13

require golang.org/x/sys v0.0.0

// A copy of the parts of golang.org/x/sys the manager uses (Windows service, processes): it builds offline, with no
// download. To use the published module instead, remove this line and run:
replace golang.org/x/sys => ./third_party/golang.org/x/sys
