import grpc
from concurrent import futures

import model.grpc_autogen.model_pb2 as pb
import model.grpc_autogen.model_pb2_grpc as pb_grpc


class PythonModelServer(pb_grpc.ModelServiceServicer):
    def DeliverModel(self, request, context):
        dst = f"/app/shared/remote-models/{request.model_id}"
        with open(dst, "wb") as f:
            f.write(request.content)

        print(f"Python server: received model {request.model_id}")
        return pb.ModelResponse(ok=True, message="Python stored model")

    # Python is not supposed to accept UploadModel, but must implement it
    def UploadModel(self, request, context):
        return pb.ModelResponse(ok=True, message="Python got UploadModel")


def run():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=4))
    pb_grpc.add_ModelServiceServicer_to_server(PythonModelServer(), server)
    server.add_insecure_port("[::]:50052")
    print("Python gRPC server listening on :50052")
    server.start()
    server.wait_for_termination()
