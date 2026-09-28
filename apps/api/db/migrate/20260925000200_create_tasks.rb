class CreateTasks < ActiveRecord::Migration[8.0]
  def change
    create_table :tasks do |t|
      t.references :repository, null: false, foreign_key: true
      t.text :user_request, null: false
      t.string :state, null: false, default: "created"
      t.string :workspace_path
      t.string :base_revision
      t.text :plan
      t.text :final_message
      t.text :error_message
      t.integer :tool_calls_count, null: false, default: 0
      t.jsonb :transcript, null: false, default: []
      t.string :resolution
      t.datetime :applied_at
      t.timestamps
    end
    add_index :tasks, :state
  end
end
