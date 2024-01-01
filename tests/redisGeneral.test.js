
import { expect, test } from "bun:test";
const redisGeneral = require('models/redisGeneral.js');


test("redisGeneral", async () => {

  const testKey = 'testKey';
  const testValue = 'testValue';

  await redisGeneral.set(testKey, testValue);

  const value = await redisGeneral.get(testKey);

  expect(value).toBe(testValue);
  
});
