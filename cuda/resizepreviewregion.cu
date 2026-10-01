#include <math.h>
// Crop BEFORE resampling. Integrate fractional cell coverage on all three axes.
extern "C" __global__ void
resizepreviewregion(float* __restrict__ dst, float* __restrict__ src,
 int Dx,int Dy,int Dz,int Sx,int Sy,int Sz,int Ox,int Oy,int Oz,int Rx,int Ry,int Rz) {
 int ix=blockIdx.x*blockDim.x+threadIdx.x;
 int iy=blockIdx.y*blockDim.y+threadIdx.y;
 int iz=blockIdx.z*blockDim.z+threadIdx.z;
 if(ix>=Dx||iy>=Dy||iz>=Dz)return;
 // Keep bin arithmetic local to avoid losing precision at large source offsets.
 float x0=float(ix)*Rx/Dx,x1=float(ix+1)*Rx/Dx;
 float y0=float(iy)*Ry/Dy,y1=float(iy+1)*Ry/Dy;
 float z0=float(iz)*Rz/Dz,z1=float(iz+1)*Rz/Dz;
 float sum=0.0f,weight=0.0f;
 for(int z=int(floorf(z0));z<int(ceilf(z1))&&z<Rz;z++) {
 float wz=fminf(z1,float(z+1))-fmaxf(z0,float(z));
 for(int y=int(floorf(y0));y<int(ceilf(y1))&&y<Ry;y++) {
 float wy=wz*(fminf(y1,float(y+1))-fmaxf(y0,float(y)));
 for(int x=int(floorf(x0));x<int(ceilf(x1))&&x<Rx;x++) {
 float w=wy*(fminf(x1,float(x+1))-fmaxf(x0,float(x)));
 if(w<=0)continue;
 sum+=src[((Oz+z)*Sy+Oy+y)*Sx+Ox+x]*w;weight+=w;
 }}}
 dst[(iz*Dy+iy)*Dx+ix]=sum/weight;
}
