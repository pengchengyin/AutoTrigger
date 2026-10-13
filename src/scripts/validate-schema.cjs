const fs = require('fs');
const path = require('path');
const Ajv2020 = require(process.env.AT_AJV_PATH || 'ajv/dist/2020');
const addFormats = require(process.env.AT_AJV_FORMATS_PATH || 'ajv-formats');
const schema = JSON.parse(fs.readFileSync(path.join(__dirname, '../pkg/schema/result.schema.json'), 'utf8'));
const ajv = new Ajv2020({ allErrors: true, strict: true });
addFormats(ajv);
const validate = ajv.compile(schema);
if (process.argv.length < 3) throw new Error('Usage: node src/scripts/validate-schema.cjs result.json [...]');
for (const file of process.argv.slice(2)) {
  const result = JSON.parse(fs.readFileSync(file, 'utf8'));
  if (!validate(result)) throw new Error(`${file}: ${JSON.stringify(validate.errors)}`);
  const count = Object.values(result.summary.by_status).reduce((a, b) => a + b, 0);
  if (count !== result.summary.total || count !== result.results.length) throw new Error(`${file}: inconsistent summary buckets`);
  process.stderr.write(`Schema and summary validated: ${file}\n`);
}
