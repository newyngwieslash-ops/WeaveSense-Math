import { detectDataSourceFormatFromFilename, parseDataSourceNode } from "@/lib/canvas/dataflow-evaluator";
import { CanvasNodeType, type CanvasDataSourceConfig } from "@/types/canvas";
import { useUserStore } from "@/stores/use-user-store";

type DataSourceFileKind = "csv" | "tsv" | "json" | "jsonl";
const MIME_KINDS: Record<string, DataSourceFileKind> = {
    "text/csv": "csv",
    "application/csv": "csv",
    "text/tab-separated-values": "tsv",
    "application/json": "json",
    "application/jsonl": "jsonl",
    "application/x-jsonlines": "jsonl",
    "application/ndjson": "jsonl",
    "application/x-ndjson": "jsonl",
};

export function dataSourceFileKind(file: Pick<File, "name" | "type">): DataSourceFileKind | undefined {
    const extension = /\.([^.]+)$/.exec(file.name.trim().toLowerCase())?.[1];
    if (extension === "csv" || extension === "tsv" || extension === "json" || extension === "jsonl") return extension;
    // Explicit text filenames keep the existing Text upload contract.
    if (extension === "txt" || extension === "md" || extension === "markdown") return undefined;
    return MIME_KINDS[file.type.split(";", 1)[0].trim().toLowerCase()];
}

export async function readDataSourceFile(file: File): Promise<CanvasDataSourceConfig> {
    const limitMB = useUserStore.getState().runtimeLimits.resourceUploadMB;
    if (!Number.isFinite(limitMB) || limitMB <= 0) throw new Error("上传大小限制不可用，请刷新后重试");
    // Match the backend's exclusive resource limit, not its chunking threshold.
    if (file.size >= limitMB * 1024 * 1024) throw new Error(`单个上传文件必须小于 ${limitMB}MB`);
    if (file.size <= 0) throw new Error("上传文件不能为空");
    const rawText = await file.text();
    const kind = dataSourceFileKind(file);
    // The evaluator selects TSV / JSONL by filename; preserve that discriminator
    // for MIME-only downloads without changing the original resource filename.
    const fileName = kind && !file.name.trim().toLowerCase().endsWith(`.${kind}`) ? `${file.name}.${kind}` : file.name;
    const dataSource: CanvasDataSourceConfig = {
        format: detectDataSourceFormatFromFilename(fileName, rawText),
        rawText,
        fileName,
    };
    const parsed = parseDataSourceNode({
        id: "file-validation",
        title: file.name,
        type: CanvasNodeType.DataSource,
        position: { x: 0, y: 0 },
        width: 0,
        height: 0,
        metadata: { dataSource },
    });
    if (parsed.error) throw new Error(parsed.error);
    return dataSource;
}
