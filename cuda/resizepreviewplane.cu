#include <math.h>
// Area-weighted in-plane reduction. A negative layer averages every cell along
// the perpendicular axis, including zero-valued cells, preserving signed fields.
extern "C" __global__ void
resizepreviewplane(float* __restrict__ dst, float* __restrict__ src,
 int Du,int Dv,int Su,int Sv,int Sn,int strideU,int strideV,int strideN,int layer) {
 int u=blockIdx.x*blockDim.x+threadIdx.x;
 int v=blockIdx.y*blockDim.y+threadIdx.y;
 if(u>=Du||v>=Dv)return;
 double u0=double(u)*Su/Du,u1=double(u+1)*Su/Du;
 double v0=double(v)*Sv/Dv,v1=double(v+1)*Sv/Dv;
 double sum=0.0,weight=0.0;
 int first=layer<0?0:layer,last=layer<0?Sn:layer+1;
 for(int n=first;n<last;n++) {
  for(int j=int(floor(v0));j<int(ceil(v1))&&j<Sv;j++) {
   double wv=fmin(v1,double(j+1))-fmax(v0,double(j));
   for(int i=int(floor(u0));i<int(ceil(u1))&&i<Su;i++) {
    double w=wv*(fmin(u1,double(i+1))-fmax(u0,double(i)));
    if(w<=0.0)continue;
    sum+=double(src[i*strideU+j*strideV+n*strideN])*w;weight+=w;
   }
  }
 }
 dst[v*Du+u]=float(sum/weight);
}
