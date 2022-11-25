# Commands

## Retrieve
Retrieve a file from the Filecoin network.

## Usage
`filctl retrieve [flags] <CID>`

## Flags
- `-m` Storage Provider to make the retrieval from
- `-o` Location to export retrieved data to. If unspecified, data will remain in the blockstore.
- `-car` If true, file will be exported in CAR format

### Example 
`filctl retrieve -m f0123 -o ./test.car -car bafy1234`