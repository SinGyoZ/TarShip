import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  addToast,
  Button,
  Card,
  CardBody,
  Chip,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  Spinner,
  Table,
  TableBody,
  TableCell,
  TableColumn,
  TableHeader,
  TableRow,
  useDisclosure,
} from '@heroui/react';
import { api, type CreateServerResult, type ServerRow } from '../api';

function formatTime(iso: string): string {
  if (!iso) return '从未连接';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const now = new Date();
  // 今天的心跳只显示时分秒，历史显示「月-日 时:分」，避免长日期撑破列宽
  const sameDay =
    d.getFullYear() === now.getFullYear() &&
    d.getMonth() === now.getMonth() &&
    d.getDate() === now.getDate();
  if (sameDay) return d.toLocaleTimeString('zh-CN', { hour12: false });
  return `${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
}

// 复制到剪贴板，回退处理非安全上下文（HTTP 部署）下的降级提示
async function copyText(text: string, label: string) {
  try {
    await navigator.clipboard.writeText(text);
    addToast({ title: `${label}已复制`, color: 'success' });
  } catch {
    addToast({ title: '复制失败', description: '请手动选中文本复制', color: 'warning' });
  }
}

// 一次性凭证展示：token 仅此一次可见，关闭后不可再取回
function TokenResultModal({
  result,
  onclose,
}: {
  result: CreateServerResult;
  onclose: () => void;
}) {
  return (
    <Modal isOpen onClose={onclose} size="2xl" backdrop="blur">
      <ModalContent>
        <ModalHeader className="flex flex-col gap-1">
          服务器「{result.name}」接入凭证
          <span className="text-sm font-normal text-warning">
            Token 仅此一次展示，请立即复制保存；关闭后无法再次查看。
          </span>
        </ModalHeader>
        <ModalBody className="gap-4">
          <div>
            <p className="mb-1 text-sm font-medium">Agent Token</p>
            <div className="flex items-center gap-2">
              <code className="min-w-0 flex-1 overflow-x-auto whitespace-nowrap rounded-lg bg-default-100 px-3 py-2 text-sm">
                {result.token}
              </code>
              <Button size="sm" variant="flat" onPress={() => copyText(result.token, 'Token ')}>
                复制
              </Button>
            </div>
          </div>
          <div>
            <div className="mb-1 flex items-center justify-between">
              <p className="text-sm font-medium">一键安装命令（目标服务器 root 执行）</p>
              <Button
                size="sm"
                variant="flat"
                color="primary"
                onPress={() => copyText(result.install_command, '安装命令')}
              >
                复制安装命令
              </Button>
            </div>
            <pre className="max-h-64 overflow-auto whitespace-pre rounded-lg bg-default-100 p-3 text-xs leading-relaxed">
              {result.install_command}
            </pre>
          </div>
        </ModalBody>
        <ModalFooter>
          <Button variant="flat" onPress={onclose}>
            已保存，关闭
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
}

export default function ServersPage() {
  const queryClient = useQueryClient();
  const addModal = useDisclosure();
  const [name, setName] = useState('');
  const [tags, setTags] = useState('');
  const [hubUrl, setHubUrl] = useState(location.origin);
  const [arch, setArch] = useState<'amd64' | 'arm64'>('amd64');
  const [tokenResult, setTokenResult] = useState<CreateServerResult | null>(null);
  const [deleting, setDeleting] = useState<ServerRow | null>(null);

  const servers = useQuery({
    queryKey: ['servers'],
    queryFn: () => api<ServerRow[]>('/api/servers'),
    refetchInterval: 30_000, // WS 事件之外的兜底轮询
  });

  const createServer = useMutation({
    mutationFn: () =>
      api<CreateServerResult>('/api/servers', {
        method: 'POST',
        body: JSON.stringify({ name, tags, hub_url: hubUrl, arch }),
      }),
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: ['servers'] });
      addModal.onClose();
      setName('');
      setTags('');
      setTokenResult(res);
    },
    onError: (e: Error) => addToast({ title: '创建失败', description: e.message, color: 'danger' }),
  });

  const deleteServer = useMutation({
    mutationFn: (id: number) =>
      api<{ ok: boolean }>(`/api/servers/${id}`, { method: 'DELETE' }),
    onSuccess: () => {
      setDeleting(null);
      queryClient.invalidateQueries({ queryKey: ['servers'] });
      addToast({ title: '服务器已删除', color: 'success' });
    },
    onError: (e: Error) => addToast({ title: '删除失败', description: e.message, color: 'danger' }),
  });

  const resetToken = useMutation({
    mutationFn: (row: ServerRow) =>
      api<{ token: string; install_command: string }>(`/api/servers/${row.id}/reset-token`, {
        method: 'POST',
        body: JSON.stringify({ hub_url: hubUrl, arch }),
      }),
    onSuccess: (res, row) => {
      queryClient.invalidateQueries({ queryKey: ['servers'] });
      setTokenResult({ id: row.id, name: row.name, token: res.token, install_command: res.install_command });
    },
    onError: (e: Error) => addToast({ title: '重置失败', description: e.message, color: 'danger' }),
  });

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold">服务器</h1>
          <p className="mt-1 text-sm text-foreground-500">
            状态实时刷新：Agent 心跳 30 秒，超 90 秒无消息判定离线
          </p>
        </div>
        <Button color="primary" onPress={addModal.onOpen}>
          添加服务器
        </Button>
      </div>

      <Card shadow="sm">
        <CardBody className="overflow-x-auto p-0">
          {servers.isPending ? (
            <div className="flex items-center justify-center py-16">
              <Spinner label="加载服务器列表…" />
            </div>
          ) : servers.isError ? (
            <p className="py-10 text-center text-sm text-danger">加载失败：{servers.error.message}</p>
          ) : !servers.data?.length ? (
            <div className="flex flex-col items-center gap-3 py-16 text-foreground-500">
              <p className="text-sm">还没有服务器接入</p>
              <Button size="sm" variant="flat" color="primary" onPress={addModal.onOpen}>
                添加第一台服务器
              </Button>
            </div>
          ) : (
            <Table aria-label="服务器列表" removeWrapper>
              <TableHeader>
                <TableColumn>状态</TableColumn>
                <TableColumn>名称</TableColumn>
                <TableColumn>主机名</TableColumn>
                <TableColumn>Agent 版本</TableColumn>
                <TableColumn>标签</TableColumn>
                <TableColumn>最后心跳</TableColumn>
                <TableColumn align="end">操作</TableColumn>
              </TableHeader>
              <TableBody>
                {servers.data.map((row) => (
                  <TableRow key={row.id}>
                    <TableCell>
                      {/* 文字 + 颜色双重表达，不单靠颜色传递状态 */}
                      <Chip
                        size="sm"
                        variant="flat"
                        color={row.online ? 'success' : 'default'}
                        startContent={
                          <span
                            aria-hidden
                            className={`ml-1 h-2 w-2 rounded-full ${
                              row.online ? 'bg-success' : 'bg-default-400'
                            }`}
                          />
                        }
                      >
                        {row.online ? '在线' : '离线'}
                      </Chip>
                    </TableCell>
                    <TableCell className="whitespace-nowrap font-medium">{row.name}</TableCell>
                    <TableCell className="whitespace-nowrap text-foreground-500">{row.hostname || '-'}</TableCell>
                    <TableCell className="whitespace-nowrap">
                      {row.agent_version ? (
                        <span className="flex items-center gap-2">
                          <code className="text-xs">{row.agent_version}</code>
                        </span>
                      ) : (
                        '-'
                      )}
                    </TableCell>
                    <TableCell>
                      {row.tags ? (
                        <div className="flex flex-wrap gap-1">
                          {row.tags.split(',').map((tag) => (
                            <Chip key={tag} size="sm" variant="dot" color="primary">
                              {tag.trim()}
                            </Chip>
                          ))}
                        </div>
                      ) : (
                        '-'
                      )}
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-xs text-foreground-500">
                      {formatTime(row.last_seen)}
                    </TableCell>
                    <TableCell>
                      <div className="flex justify-end gap-2">
                        <Button
                          size="sm"
                          variant="flat"
                          onPress={() => resetToken.mutate(row)}
                          isLoading={resetToken.isPending && resetToken.variables?.id === row.id}
                        >
                          重置 Token
                        </Button>
                        <Button size="sm" variant="flat" color="danger" onPress={() => setDeleting(row)}>
                          删除
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardBody>
      </Card>

      {/* 添加服务器向导 */}
      <Modal isOpen={addModal.isOpen} onClose={addModal.onClose} backdrop="blur">
        <ModalContent>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              createServer.mutate();
            }}
          >
            <ModalHeader>添加服务器</ModalHeader>
            <ModalBody className="gap-4">
              <Input
                label="服务器名称"
                labelPlacement="outside"
                placeholder="如 vps-01"
                value={name}
                onValueChange={setName}
                isRequired
              />
              <Input
                label="标签（可选）"
                labelPlacement="outside"
                placeholder="逗号分隔，如 prod,web"
                value={tags}
                onValueChange={setTags}
              />
              <Input
                label="Hub 地址"
                labelPlacement="outside"
                placeholder="Agent 将连接此地址"
                value={hubUrl}
                onValueChange={setHubUrl}
                description="Agent 从所在服务器主动连出，此地址必须能被目标服务器访问"
                isRequired
              />
              <div>
                <p className="mb-2 text-sm text-foreground-500">目标服务器架构</p>
                <div className="flex gap-2">
                  {(['amd64', 'arm64'] as const).map((a) => (
                    <Button
                      key={a}
                      type="button"
                      size="sm"
                      variant={arch === a ? 'solid' : 'flat'}
                      color={arch === a ? 'primary' : 'default'}
                      onPress={() => setArch(a)}
                      aria-pressed={arch === a}
                    >
                      {a}
                    </Button>
                  ))}
                </div>
              </div>
            </ModalBody>
            <ModalFooter>
              <Button type="button" variant="flat" onPress={addModal.onClose}>
                取消
              </Button>
              <Button type="submit" color="primary" isLoading={createServer.isPending}>
                创建并生成 Token
              </Button>
            </ModalFooter>
          </form>
        </ModalContent>
      </Modal>

      {/* 删除确认 */}
      <Modal isOpen={deleting !== null} onClose={() => setDeleting(null)} size="sm" backdrop="blur">
        <ModalContent>
          <ModalHeader>删除服务器</ModalHeader>
          <ModalBody>
            <p className="text-sm text-foreground-500">
              确认删除「{deleting?.name}」？其 Agent 将立即失联，该操作不可撤销。
            </p>
          </ModalBody>
          <ModalFooter>
            <Button variant="flat" onPress={() => setDeleting(null)}>
              取消
            </Button>
            <Button
              color="danger"
              isLoading={deleteServer.isPending}
              onPress={() => deleting && deleteServer.mutate(deleting.id)}
            >
              确认删除
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {tokenResult && <TokenResultModal result={tokenResult} onclose={() => setTokenResult(null)} />}
    </div>
  );
}
