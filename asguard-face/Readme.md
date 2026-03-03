# Asguard Face Service

Privacy-first face recognition microservice. Stateless — we process, you store.

## Quick Start

```bash
# 1. Download dlib models (one-time)
mkdir -p models
wget -O models/shape_predictor_5_face_landmarks.dat https://github.com/davisking/dlib-models/raw/master/shape_predictor_5_face_landmarks.dat.bz2
bunzip2 models/shape_predictor_5_face_landmarks.dat.bz2

wget -O models/dlib_face_recognition_resnet_model_v1.dat https://github.com/davisking/dlib-models/raw/master/dlib_face_recognition_resnet_model_v1.dat.bz2
bunzip2 models/dlib_face_recognition_resnet_model_v1.dat.bz2

# 2. Run
docker-compose up asguard-face
