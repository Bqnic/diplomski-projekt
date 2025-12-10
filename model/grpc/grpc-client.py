import grpc
import model.grpc_autogen.model_pb2 as pb
import model.grpc_autogen.model_pb2_grpc as pb_grpc


def send_to_go(path, model_id):
    with open(path, "rb") as f:
        content = f.read()

    channel = grpc.insecure_channel("localhost:50051")
    stub = pb_grpc.ModelServiceStub(channel)

    resp = stub.UploadModel(pb.ModelRequest(
        model_id=model_id,
        content=content,
        size=len(content),
        hash=""
    ))

    print("Python client: Go replied:", resp.message)
