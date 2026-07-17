extern "C" __global__ void
oerstedkernmul3d(float* __restrict__ fftJx, float* __restrict__ fftJy, float* __restrict__ fftJz,
                 float* __restrict__ fftKx, float* __restrict__ fftKy, float* __restrict__ fftKz,
                 int Nx, int Ny, int Nz) {
    int ix = blockIdx.x * blockDim.x + threadIdx.x;
    int iy = blockIdx.y * blockDim.y + threadIdx.y;
    int iz = blockIdx.z * blockDim.z + threadIdx.z;
    if (ix >= Nx || iy >= Ny || iz >= Nz) return;
    int e = 2 * ((iz * Ny + iy) * Nx + ix);
    float ax = fftJx[e], bx = fftJx[e+1];
    float ay = fftJy[e], by = fftJy[e+1];
    float az = fftJz[e], bz = fftJz[e+1];
    float cx = fftKx[e], dx = fftKx[e+1];
    float cy = fftKy[e], dy = fftKy[e+1];
    float cz = fftKz[e], dz = fftKz[e+1];
    fftJx[e]   = (ay*cz-by*dz) - (az*cy-bz*dy);
    fftJx[e+1] = (ay*dz+by*cz) - (az*dy+bz*cy);
    fftJy[e]   = (az*cx-bz*dx) - (ax*cz-bx*dz);
    fftJy[e+1] = (az*dx+bz*cx) - (ax*dz+bx*cz);
    fftJz[e]   = (ax*cy-bx*dy) - (ay*cx-by*dx);
    fftJz[e+1] = (ax*dy+bx*cy) - (ay*dx+by*cx);
}
