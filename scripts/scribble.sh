sudo apt update && sudo apt install wget xz-utils gcc g++ git make gcc-aarch64-linux-gnu gcc-s390x-linux-gnu gcc-riscv64-linux-gnu g++-aarch64-linux-gnu g++-riscv64-linux-gnu -y

./CLI_Compile.sh

rm -rf Shared/Source/zlib
#Arm 64
export CC=$(which aarch64-linux-gnu-gcc)
export CXX=$(which aarch64-linux-gnu-g++)
./CLI_Compile.sh --host aarch64-linux-gnu

rm -rf Shared/Source/zlib

#riscv64
export CC=$(which riscv64-linux-gnu-gcc)
export CXX=$(which riscv64-linux-gnu-g++)
./CLI_Compile.sh --host riscv64-linux-gnu