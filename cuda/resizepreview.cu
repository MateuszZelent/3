#include <math.h>
// Fused volume resize: one launch per component, independent of layer count.
// Fractional coverage weights preserve averages for prime/non-divisible XY sizes.
extern "C" __global__ void
resizepreview(float* __restrict__ dst, float* __restrict__ src,
 int Dx,int Dy,int Dz,int Sx,int Sy,int Sz,int layer,int stride) {
 int ix=blockIdx.x*blockDim.x+threadIdx.x;
 int iy=blockIdx.y*blockDim.y+threadIdx.y;
 int iz=blockIdx.z*blockDim.z+threadIdx.z;
 if(ix>=Dx||iy>=Dy||iz>=Dz)return;
 int sourceZ=layer>=0?layer:iz*stride+stride/2;
 sourceZ=sourceZ<Sz?sourceZ:Sz-1;
 float x0=float(ix)*Sx/Dx,x1=float(ix+1)*Sx/Dx;
 float y0=float(iy)*Sy/Dy,y1=float(iy+1)*Sy/Dy;
 float selected=nanf("");
 int first=layer==-2?0:sourceZ,last=layer==-2?Sz:sourceZ+1;
 for(int source=first;source<last;source++) {
 float sum=0.0f,weight=0.0f;
 for(int y=int(floorf(y0));y<int(ceilf(y1))&&y<Sy;y++) {
  float wy=fminf(y1,float(y+1))-fmaxf(y0,float(y));
  for(int x=int(floorf(x0));x<int(ceilf(x1))&&x<Sx;x++) {
   float w=wy*(fminf(x1,float(x+1))-fmaxf(x0,float(x)));
   sum+=src[(source*Sy+y)*Sx+x]*w;weight+=w;
  }
 }
 float value=sum/weight;
 if(isfinite(value) && (!isfinite(selected)||fabsf(value)>fabsf(selected)))selected=value;
 }
 dst[(iz*Dy+iy)*Dx+ix]=selected;
}
