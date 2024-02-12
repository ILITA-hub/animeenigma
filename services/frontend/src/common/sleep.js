export default async function sleep(ms) {
  await new Promise((resolve, reject) => {
    setTimeout(() => {
      resolve()
    }, ms);
  })
}
