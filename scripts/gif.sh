#!/usr/bin/env bash

# Generate a GIF from the tape and optionally upload it to vhs.charm.sh

cd _examples/app

vhs < app.tape

echo "GIF generated and moved to the root directory as app.gif"

read -p "Do you want to upload the GIF to vhs.charm.sh? (y/n): " upload

if [[ "$upload" == "y" || "$upload" == "Y" ]]; then
    echo "Uploading the GIF to vhs.charm.sh"
    vhs publish ./app.gif | tee gif.log
else
    echo "GIF not uploaded."
fi