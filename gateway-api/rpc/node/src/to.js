export default function to(promise) {
  if (!(promise instanceof Promise)) {
    promise = Promise.resolve(promise);
  }
  return promise.then(data => [ null, data ]).catch(err => [ err ]);
}
