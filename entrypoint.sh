#!/bin/sh

# Clone or pull the repository
if [ -d "$CLONE_DIR/.git" ]; then
  git -C "$CLONE_DIR" pull origin main
else
  git clone "$REPO_URL" "$CLONE_DIR"
fi

# Install dependencies and build
cd "$CLONE_DIR"
npm install
npm run build

# Copy files to the volume-mounted directory
mkdir -p "$BUILD_DIR"
cp -r "$CLONE_DIR/dist"/* "$BUILD_DIR"/

# Explicitly exit to ensure the container stops
exit 0