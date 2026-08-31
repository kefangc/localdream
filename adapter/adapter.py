"""Local bridge from Local Dream jobs to ComfyUI's API prompt graph."""
import json, os, shutil, uuid, time, threading
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

try:
    import websocket
except ImportError:
    websocket = None

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
COMFY_URL = os.getenv("COMFYUI_URL", "http://127.0.0.1:8188").rstrip("/")
WORKFLOW = os.path.join(ROOT, "templates", "h3", "workflow.json")
MANIFEST = os.path.join(ROOT, "templates", "h3", "manifest.json")
ASSET_DIR = os.path.join(ROOT, "data", "assets")
COMFY_INPUT = os.getenv("COMFYUI_INPUT", r"D:\Comfy-Desktop\ComfyUI-Shared\input")
TASKS = {}
TASKS_LOCK = threading.Lock()
IMAGE_EXTENSIONS = {".png", ".jpg", ".jpeg", ".webp", ".bmp", ".gif"}
VIDEO_EXTENSIONS = {".mp4", ".mov", ".mkv", ".webm", ".avi", ".m4v"}

def comfy_json(path, payload=None):
    raw = json.dumps(payload).encode() if payload is not None else None
    req = Request(COMFY_URL + path, data=raw, headers={"Content-Type":"application/json"} if raw else {})
    with urlopen(req, timeout=15) as response: return json.loads(response.read() or b"{}")

def wait_result(prompt_id):
    for _ in range(1800):
        try:
            history=comfy_json("/history/"+prompt_id)
            item=history.get(prompt_id)
            if item:
                if item.get("status",{}).get("status_str")=="error":
                    return {"status":"failed","error":item.get("status",{}).get("messages",[]) }
                for output in item.get("outputs",{}).values():
                    videos = output.get("gifs",[]) + output.get("videos",[])
                    # SaveVideo currently reports media under `images`, even for MP4/WebM.
                    videos += [item for item in output.get("images",[]) if str(item.get("filename","")).lower().endswith((".mp4", ".webm", ".mov", ".mkv"))]
                    for video in videos:
                        from urllib.parse import quote
                        filename=quote(str(video.get("filename", "")), safe="")
                        subfolder=quote(str(video.get("subfolder", "")), safe="")
                        media_type=quote(str(video.get("type", "output")), safe="")
                        return {"status":"completed","output_url":COMFY_URL+"/view?filename="+filename+"&subfolder="+subfolder+"&type="+media_type}
                return {"status":"completed","output_url":""}
        except Exception:
            pass
        time.sleep(1)
    return {"status":"failed","error":"等待 ComfyUI 结果超时"}

def update_task(prompt_id, **values):
    with TASKS_LOCK:
        TASKS.setdefault(prompt_id, {}).update(values)

def watch_prompt(prompt_id, client_id):
    update_task(prompt_id, status="queued", progress=5)
    if websocket is not None:
        ws = None
        try:
            ws_url = COMFY_URL.replace("https://", "wss://").replace("http://", "ws://") + "/ws?clientId=" + client_id
            ws = websocket.create_connection(ws_url, timeout=5)
            ws.settimeout(5)
            while True:
                try:
                    raw = ws.recv()
                except Exception:
                    break
                if not raw or isinstance(raw, bytes):
                    continue
                try:
                    message = json.loads(raw)
                except json.JSONDecodeError:
                    continue
                data = message.get("data", {})
                if data.get("prompt_id") != prompt_id:
                    continue
                message_type = message.get("type")
                if message_type == "progress":
                    maximum = max(1, int(data.get("max", 1)))
                    value = min(max(0, int(data.get("value", 0))), maximum)
                    update_task(prompt_id, status="running", progress=20 + round(value / maximum * 75))
                elif message_type == "executing" and data.get("node") is not None:
                    update_task(prompt_id, status="running", progress=20)
                elif message_type == "executing" and data.get("node") is None:
                    break
        except Exception:
            pass
        finally:
            if ws is not None:
                try: ws.close()
                except Exception: pass
    final = wait_result(prompt_id)
    update_task(prompt_id, **final)

def inside(node, group):
    if not group or not node.get("pos") or not group.get("bounding"): return False
    try:
        x,y=node["pos"]
        bounding=group["bounding"]
        if isinstance(bounding,(list,tuple)):
            gx,gy,gw,gh=map(float,bounding)
        else:
            gx,gy,gw,gh=map(float,str(bounding).replace("[","").replace("]","").replace(","," ").split())
        return gx<=x<=gx+gw and gy<=y<=gy+gh
    except (ValueError,TypeError): return False

def resolution_inputs(ratio, resolution):
    ratios={"16:9":"16:9 (Widescreen)", "9:16":"9:16 (Portrait Widescreen)", "1:1":"1:1 (Square)", "4:3":"4:3 (Standard)"}
    # ResolutionSelector derives dimensions with a 32px multiple. 0.4 MP is
    # the template's 480p-class setting; 0.9 MP is its 720p-class setting.
    return ratios.get(ratio, "16:9 (Widescreen)"), 0.4 if resolution=="480p" else 0.9

def copied_assets(asset_ids):
    assets=[]
    if not asset_ids: return assets
    if not os.path.isdir(ASSET_DIR): raise ValueError("本地素材目录不存在")
    os.makedirs(COMFY_INPUT,exist_ok=True)
    for asset_id in asset_ids:
        prefix=str(asset_id)+"."
        filename=next((name for name in os.listdir(ASSET_DIR) if name.startswith(prefix)),None)
        if not filename: raise ValueError("找不到素材: "+str(asset_id))
        ext=os.path.splitext(filename)[1].lower()
        kind="image" if ext in IMAGE_EXTENSIONS else "video" if ext in VIDEO_EXTENSIONS else "unknown"
        if kind=="unknown": raise ValueError("不支持的素材格式: "+filename)
        shutil.copy2(os.path.join(ASSET_DIR,filename),os.path.join(COMFY_INPUT,filename))
        assets.append({"id":str(asset_id),"filename":filename,"kind":kind})
    return assets

def attach_assets(graph, info, mode, assets):
    images=[asset["filename"] for asset in assets if asset["kind"]=="image"]
    videos=[asset["filename"] for asset in assets if asset["kind"]=="video"]
    if mode=="text_to_video":
        if assets: raise ValueError("文生视频不接受参考素材，请选择图生、首尾帧或多参考视频")
        return
    if mode=="image_to_video":
        if videos or len(images)!=1: raise ValueError("图生视频需要且只能使用 1 张图片")
    elif mode=="first_last_frame":
        if videos or len(images)!=2: raise ValueError("首尾帧模式需要且只能使用 2 张图片")
    elif mode=="multi_reference_video":
        if not assets: raise ValueError("多参考视频至少需要 1 个图片或视频素材")
        if len(images)>9 or len(videos)>3: raise ValueError("多参考最多支持 9 张图片和 3 个视频")
    else: raise ValueError("未知模式: "+mode)

    target=graph[str(info["prompt_node"])]
    if mode!="multi_reference_video":
        for node_id,filename in zip(info.get("asset_nodes",[]),images):
            node=graph.get(str(node_id))
            if not node or node["class_type"]!="LoadImage": raise ValueError("工作流图片输入节点缺失: "+str(node_id))
            node["inputs"]["image"]=filename
        return

    for key in list(target["inputs"]):
        if key.startswith(("ref_images.","ref_videos.","ref_video_audios.","ref_audios.")): target["inputs"].pop(key)
    static_nodes=list(info.get("asset_nodes",[]))
    next_id=max([int(node_id) for node_id in graph if str(node_id).isdigit()]+[0])+1
    for index,filename in enumerate(images):
        if index<len(static_nodes) and str(static_nodes[index]) in graph:
            node_id=str(static_nodes[index]); graph[node_id]["inputs"]["image"]=filename
        else:
            node_id=str(next_id); next_id+=1
            graph[node_id]={"class_type":"LoadImage","inputs":{"image":filename},"_meta":{"title":"动态参考图片"}}
        target["inputs"]["ref_images.ref_image_"+str(index)]=[node_id,0]
    for index,filename in enumerate(videos):
        node_id=str(next_id); next_id+=1
        graph[node_id]={"class_type":"VHS_LoadVideo","inputs":{"video":filename,"force_rate":0,"custom_width":0,"custom_height":0,"frame_load_cap":0,"skip_first_frames":0,"select_every_nth":1},"_meta":{"title":"动态参考视频帧"}}
        target["inputs"]["ref_videos.ref_video_"+str(index)]=[node_id,0]

def apply_acceleration(graph, info, payload):
    selection=str(payload.get("acceleration") or payload.get("lora") or "").strip()
    options=info.get("accelerations",{})
    if selection and selection not in options: raise ValueError("该模式不支持所选 LoRA 加速: "+selection)
    option=options.get(selection)
    steps=max(1,min(100,int(payload.get("steps",4))))
    if option and steps!=int(option["steps"]):
        raise ValueError(str(option["label"])+" 必须使用 "+str(option["steps"])+" 步；取消加速后才能自定义步数")
    for node in graph.values():
        if node["class_type"]=="BasicScheduler":
            node["inputs"].update({"scheduler":"simple","steps":steps,"denoise":1})
        if node["class_type"]=="TESpeedMiniMaxH3":
            node["inputs"]["mode"]=(option or {}).get("te_speed_mode","standard")
    bypass={}
    for node_id,node in graph.items():
        if node["class_type"]!="LoraLoaderModelOnly": continue
        if option:
            node["inputs"]["lora_name"]=option["lora_name"]
            node["inputs"]["strength_model"]=max(0.0,min(1.0,float(payload.get("lora_strength",0.75))))
        elif isinstance(node["inputs"].get("model"),list): bypass[node_id]=node["inputs"]["model"]
    for node in graph.values():
        for key,value in list(node["inputs"].items()):
            if isinstance(value,list) and len(value)>=2 and str(value[0]) in bypass: node["inputs"][key]=bypass[str(value[0])]
    for node_id in bypass: graph.pop(node_id,None)

def ui_to_api(payload):
    with open(WORKFLOW,encoding="utf-8") as f: doc=json.load(f)
    with open(MANIFEST,encoding="utf-8") as f: manifest=json.load(f)
    mode=payload.get("mode","image_to_video"); info=manifest.get("modes",{}).get(mode,{})
    if not info:
        raise ValueError("不支持的 H3 模式: "+str(mode))
    groups=doc.get("groups",[])
    target=next((group for group in groups if str(group.get("id"))==str(info.get("group_id"))),None)
    group_index=info.get("group_index")
    if target is None and group_index is not None and int(group_index)<len(groups): target=groups[int(group_index)]
    if target is None:
        raise ValueError("H3 工作流缺少模式分组: "+str(mode))
    nodes={str(n["id"]):n for n in doc.get("nodes",[])}; links={str(x[0]):x for x in doc.get("links",[])}
    ignored={"Note","MarkdownNote","Label (rgthree)"}
    # Each H3 mode is an independent group. Do not include other active groups;
    # their mode=0 nodes otherwise make every request execute the multi-reference branch.
    selected={nid for nid,n in nodes.items() if n.get("type") not in ignored and inside(n,target)}
    if not selected:
        raise ValueError("H3 模式分组没有可执行节点: "+str(mode))
    changed=True
    while changed:
        changed=False
        for nid in list(selected):
            for item in nodes[nid].get("inputs",[]) or []:
                link=item.get("link")
                if link is not None and str(link) in links:
                    src=str(links[str(link)][1])
                    if src not in selected and nodes.get(src,{}).get("type") not in ignored: selected.add(src); changed=True
    graph={}
    for nid in selected:
        n=nodes[nid]; inputs={}
        for item in n.get("inputs",[]) or []:
            link=item.get("link")
            if link is not None and str(link) in links:
                x=links[str(link)]; inputs[item["name"]]=[str(x[1]),int(x[2])]
        widgets=list(n.get("widgets_values") or [])
        try:
            # Canvas exports keep widget values for inputs that are currently linked.
            # Consume them according to the node's declared widget order, retaining links.
            widget_names=[item.get("name") for item in n.get("inputs",[]) or [] if item.get("widget")]
            for name in widget_names:
                if not widgets: break
                value=widgets.pop(0); value=value[0] if isinstance(value,list) and len(value)==1 else value
                if name not in inputs: inputs[name]=value
            required=comfy_json("/object_info/"+n["type"]).get(n["type"],{}).get("input",{}).get("required",{})
            for name in required:
                if name in inputs or not widgets: continue
                value=widgets.pop(0); inputs[name]=value[0] if isinstance(value,list) and len(value)==1 else value
        except Exception: pass
        graph[nid]={"class_type":n["type"],"inputs":inputs,"_meta":{"title":n.get("title") or n["type"]}}
    for nid,node in graph.items():
        if str(nid)==str(info.get("prompt_node")):
            node["inputs"]["prompt"]=payload.get("prompt","")
    unet_node=info.get("unet_node")
    if unet_node is not None and str(unet_node) in graph and info.get("unet_model"):
        graph[str(unet_node)]["inputs"].update({"unet_name":info["unet_model"],"weight_dtype":"default"})
    duration_node=graph.get(str(info.get("duration_node")))
    if duration_node: duration_node["inputs"]["value"]=float(payload.get("duration",5))
    resolution_node=graph.get(str(info.get("resolution_node")))
    if resolution_node:
        aspect,megapixels=resolution_inputs(payload.get("aspect_ratio","16:9"),payload.get("resolution","720p"))
        resolution_node["inputs"].update({"aspect_ratio":aspect,"megapixels":megapixels,"multiple":32})
    apply_acceleration(graph,info,payload)
    attach_assets(graph,info,mode,copied_assets(payload.get("asset_ids",[])))
    return {"prompt":graph,"client_id":"localdream-"+uuid.uuid4().hex}

class Handler(BaseHTTPRequestHandler):
    def log_message(self,fmt,*args): pass
    def do_POST(self):
        if self.path!="/api/localdream/submit": self.send_error(404); return
        try:
            length=int(self.headers.get("Content-Length","0")); payload=json.loads(self.rfile.read(length) or b"{}"); compiled=ui_to_api(payload); client_id=compiled["client_id"]; result=comfy_json("/prompt",compiled); prompt_id=result.get("prompt_id","")
            if not prompt_id: raise RuntimeError("ComfyUI 未返回 prompt_id")
            with TASKS_LOCK: TASKS[prompt_id]={"status":"queued","progress":5}
            threading.Thread(target=watch_prompt,args=(prompt_id,client_id),daemon=True).start()
            self.reply(202,{"accepted":True,"prompt_id":prompt_id,"status":"queued","progress":5})
        except HTTPError as exc: self.reply(502,{"error":"ComfyUI /prompt rejected the compiled workflow","detail":exc.read().decode("utf-8","replace")})
        except (URLError,OSError) as exc: self.reply(502,{"error":"ComfyUI is unreachable","detail":str(exc)})
        except Exception as exc: self.reply(500,{"error":"workflow compilation failed","detail":str(exc)})
    def do_GET(self):
        prefix="/api/localdream/status/"
        if not self.path.startswith(prefix): self.send_error(404); return
        prompt_id=self.path[len(prefix):].split("?",1)[0]
        if not prompt_id: self.send_error(400); return
        with TASKS_LOCK: state=dict(TASKS.get(prompt_id, {"status":"queued","progress":5}))
        if state.get("status") not in ("completed", "failed"):
            try:
                history=comfy_json("/history/"+prompt_id)
                item=history.get(prompt_id)
                if item and item.get("status",{}).get("status_str")=="error":
                    state={"status":"failed","progress":state.get("progress",20),"error":str(item.get("status",{}).get("messages",[]))}
                    update_task(prompt_id, **state)
                elif item and item.get("outputs"):
                    final=wait_result(prompt_id)
                    state={"progress":100, **final}
                    update_task(prompt_id, **state)
            except Exception:
                pass
        self.reply(200,state)
    def reply(self,status,data):
        raw=json.dumps(data,ensure_ascii=False).encode(); self.send_response(status); self.send_header("Content-Type","application/json; charset=utf-8"); self.send_header("Content-Length",str(len(raw))); self.end_headers(); self.wfile.write(raw)

if __name__=="__main__": HTTPServer(("127.0.0.1",8791),Handler).serve_forever()
