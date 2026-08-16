async function run() {
  const res = await fetch('http://localhost:8088/api/v1/projects');
  const data = await res.json();
  console.log('Projects raw response:', JSON.stringify(data).slice(0, 300));
  if (data.items?.length) {
    console.log('Sample project IDs and slugs:');
    data.items.slice(0, 5).forEach(p => console.log(`id: ${p.id}, slug: ${p.slug}, title: ${p.title}`));
  }

  const sRes = await fetch('http://localhost:8088/api/v1/services');
  const sData = await sRes.json();
  console.log('Services count:', sData.items?.length);
  if (sData.items?.length) {
    sData.items.slice(0, 5).forEach(s => console.log(`id: ${s.id}, slug: ${s.slug}, title: ${s.title}`));
  }

  const vRes = await fetch('http://localhost:8088/api/v1/vacancies');
  const vData = await vRes.json();
  console.log('Vacancies count:', vData.items?.length);
  if (vData.items?.length) {
    vData.items.slice(0, 5).forEach(v => console.log(`id: ${v.id}, slug: ${v.slug}, title: ${v.title}`));
  }

  const fRes = await fetch('http://localhost:8088/api/v1/freelancers');
  const fData = await fRes.json();
  console.log('Freelancers count:', fData.items?.length);
  if (fData.items?.length) {
    fData.items.slice(0, 5).forEach(f => console.log(`id: ${f.id}, username: ${f.username}, name: ${f.full_name}`));
  }
}
run().catch(console.error);
