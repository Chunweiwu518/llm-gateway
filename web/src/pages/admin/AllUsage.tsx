import UsageTable from '../../components/UsageTable'

export default function AllUsage() {
  return (
    <>
      <h1>All usage</h1>
      <UsageTable endpoint="/api/admin/usage" showUser />
    </>
  )
}
