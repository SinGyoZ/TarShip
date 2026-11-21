import { PageHeader } from "../components/PageHeader";

export default function PlaceholderPage({ title }: { title: string }) {
  return (
    <>
      <PageHeader title={title} />
      <p className="max-w-md text-sm text-muted">这部分功能尚未开放，会在后续版本中加入。</p>
    </>
  );
}
